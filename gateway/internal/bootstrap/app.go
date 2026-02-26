// Package app fournit le câblage principal de la Gateway ZTNA.
// Il assemble le listener mTLS, le handler de protocole CONNECT,
// le client d'autorisation vers le Control Plane, le proxy TCP
// et le gestionnaire de sessions.
package app

import (
	"context"
	"fmt"
	"log/slog"

	"ztna-gateway/internal/infra/authz"
	"ztna-gateway/internal/infra/revocation"
	"ztna-gateway/internal/config"
	"ztna-gateway/internal/infra/cache"
	"ztna-gateway/internal/infra/mtls"
	"ztna-gateway/internal/usecase/connect"
	"ztna-gateway/internal/infra/proxy"
	"ztna-gateway/internal/infra/session"
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

	// Handler de protocole CONNECT
	connectHandler := protocol.NewHandler(authzClient, tcpProxy, sessionMgr, log)

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
//
// TODO: Implémenter la boucle d'acceptation des connexions
// TODO: Lancer le traitement de chaque connexion dans une goroutine
// TODO: Limiter le nombre de connexions concurrentes
func (a *App) Run(ctx context.Context) error {
	_ = ctx
	a.log.Info("démarrage du listener mTLS", "addr", a.cfg.Server.ListenAddr)
	a.log.Debug("security components initialized",
		"crl_ready", a.crl != nil,
		"decision_cache_ready", a.cache != nil,
	)

	// TODO: appeler a.listener.Listen(ctx) pour démarrer l'acceptation
	// TODO: pour chaque connexion, appeler a.handler.Handle(ctx, conn) en goroutine

	return fmt.Errorf("TODO: Run non implémenté")
}

// Close effectue l'arrêt graceful de la Gateway.
//
// TODO: Fermer le listener (plus de nouvelles connexions)
// TODO: Attendre la fin des sessions actives (avec timeout)
// TODO: Fermer les connexions proxy
func (a *App) Close(ctx context.Context) error {
	a.log.Info("arrêt graceful de la gateway")

	// TODO: fermer le listener mTLS
	// TODO: drainer les sessions actives
	// TODO: fermer le gestionnaire de sessions

	return nil
}
