// Package app fournit le câblage principal de la Gateway ZTNA.
// Il assemble le listener mTLS, le handler de protocole CONNECT,
// le client d'autorisation vers le Control Plane, le proxy TCP
// et le gestionnaire de sessions.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"ztna-gateway/internal/config"
	authorize "ztna-gateway/internal/infra/authz"
	decisioncache "ztna-gateway/internal/infra/cache"
	"ztna-gateway/internal/infra/mtls"
	"ztna-gateway/internal/infra/proxy"
	crl "ztna-gateway/internal/infra/revocation"
	"ztna-gateway/internal/infra/session"
	tlsutil "ztna-gateway/internal/infra/tls"
	protocol "ztna-gateway/internal/usecase/connect"
)

// App regroupe tous les composants nécessaires à l'exécution de la Gateway ZTNA.
type App struct {
	cfg      *config.Config
	log      *slog.Logger
	listener *mtls.Listener
	handler  *protocol.Handler
	authz    *authorize.Client
	proxy    *proxy.TCPProxy
	sessions *session.Manager
	crl      *crl.Store
	cache    *decisioncache.Cache
}

// New construit une instance App à partir de la configuration et du logger.
// Chaque composant est initialisé mais le listener n'est pas encore démarré.
func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	// Client d'autorisation vers le Control Plane
	authzClient := authorize.NewClient(cfg, log)

	// Proxy TCP pour relayer le trafic
	tcpProxy := proxy.NewTCPProxy(cfg, log)

	// Gestionnaire de sessions
	sessionMgr := session.NewManager(log)

	// Composants sécurité/résilience (préparation progressive)
	crlStore := crl.NewStore()
	decisionCache := decisioncache.New(cfg.DecisionCacheMaxKeys)

	// KillRevoked : quand la CRL est rafraîchie, toutes les sessions actives
	// dont le serial est révoqué sont immédiatement coupées.
	crlStore.SetOnRevoke(sessionMgr.KillRevoked)

	// Handler de protocole CONNECT (avec CRL store pour vérification de révocation)
	connectHandler := protocol.NewHandler(authzClient, tcpProxy, sessionMgr, crlStore, log, cfg.PEP.ID)

	// Listener mTLS
	listener, err := mtls.NewListener(cfg, connectHandler, log)
	if err != nil {
		return nil, fmt.Errorf("impossible de créer le listener mTLS: %w", err)
	}

	return &App{
		cfg:      cfg,
		log:      log,
		listener: listener,
		handler:  connectHandler,
		authz:    authzClient,
		proxy:    tcpProxy,
		sessions: sessionMgr,
		crl:      crlStore,
		cache:    decisionCache,
	}, nil
}

// Run démarre le listener mTLS et accepte les connexions entrantes.
//
// Flux de traitement pour chaque connexion :
//  1. Accepter la connexion TLS (le handshake mTLS vérifie le certificat client)
//  2. Extraire l'identité du client depuis le certificat (SubjectRef)
//  3. Lire la requête CONNECT du client
//  4. Appeler le Control Plane pour obtenir une décision d'autorisation
//  5. Si allow : établir la connexion proxy vers la ressource cible
//  6. Si deny : envoyer une réponse d'erreur et fermer la connexion
//  7. Relayer le trafic bidirectionnel
//  8. Journaliser la fin de session
func (a *App) Run(ctx context.Context) error {
	a.log.Info("démarrage du listener mTLS",
		"addr", a.cfg.Server.ListenAddr,
		"crl_ready", a.crl != nil,
		"decision_cache_ready", a.cache != nil,
	)

	// Démarrer le refresh CRL en background
	if a.crl != nil && a.cfg.ControlPlane.BaseURL != "" {
		crlInterval := a.cfg.CRLRefreshInterval
		if crlInterval <= 0 {
			crlInterval = 30 * time.Second
		}
		// HTTP client avec TLS skip verify pour le lab (même config que authz client)
		crlHTTP, err := tlsutil.NewControlPlaneHTTPClient(a.cfg, 10*time.Second)
		if err != nil || crlHTTP == nil {
			// Fallback : client sans vérification TLS
			crlHTTP = &http.Client{Timeout: 10 * time.Second}
		}
		a.crl.StartAutoRefresh(ctx, a.cfg.ControlPlane.BaseURL, crlHTTP, crlInterval, a.log)
		a.log.Info("CRL auto-refresh démarré", "interval", crlInterval)
	}

	return a.listener.Listen(ctx)
}

// Close effectue l'arrêt graceful de la Gateway.
func (a *App) Close(ctx context.Context) error {
	a.log.Info("arrêt graceful de la gateway")
	return a.listener.Close()
}
