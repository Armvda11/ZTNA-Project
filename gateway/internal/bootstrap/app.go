// Package app fournit le câblage principal de la Gateway ZTNA.
// Il assemble le listener mTLS, le handler de protocole CONNECT,
// le client d'autorisation vers le Control Plane, le proxy TCP,
// le gestionnaire de sessions, le CRL auto-refresh, le cache de
// décisions, le heartbeat et la télémétrie de session.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	authorize "ztna-gateway/internal/infra/authz"
	crl "ztna-gateway/internal/infra/revocation"
	"ztna-gateway/internal/config"
	decisioncache "ztna-gateway/internal/infra/cache"
	"ztna-gateway/internal/infra/heartbeat"
	"ztna-gateway/internal/infra/mtls"
	protocol "ztna-gateway/internal/usecase/connect"
	"ztna-gateway/internal/infra/proxy"
	resourceclient "ztna-gateway/internal/infra/resource"
	"ztna-gateway/internal/infra/session"
	"ztna-gateway/internal/infra/telemetry"
	tlsutil "ztna-gateway/internal/infra/tls"
)

// App regroupe tous les composants nécessaires à l'exécution de la Gateway ZTNA.
type App struct {
	cfg       *config.Config
	log       *slog.Logger
	listener  *mtls.Listener
	handler   *protocol.Handler
	authz     *authorize.Client
	proxy     *proxy.TCPProxy
	sessions  *session.Manager
	crl       *crl.Store
	cache     *decisioncache.Cache
	heartbeat *heartbeat.Client
	telemetry *telemetry.Client
}

// New construit une instance App à partir de la configuration et du logger.
// Chaque composant est initialisé mais le listener n'est pas encore démarré.
func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	// Client HTTP partagé pour les communications vers le CP
	cpHTTPClient, err := tlsutil.NewControlPlaneHTTPClient(cfg, 10*time.Second)
	if err != nil {
		log.Warn("client HTTP CP non initialisé — certaines fonctionnalités seront dégradées", "error", err)
	}

	// Client d'autorisation vers le Control Plane
	authzClient := authorize.NewClient(cfg, log)

	// Proxy TCP pour relayer le trafic
	tcpProxy := proxy.NewTCPProxy(cfg, log)

	// Gestionnaire de sessions
	sessionMgr := session.NewManager(log)

	// CRL store avec auto-refresh depuis le CP
	var crlStore *crl.Store
	if cpHTTPClient != nil {
		crlURL := fmt.Sprintf("%s/pki/device-ca/crl", cfg.ControlPlane.BaseURL)
		crlStore = crl.NewStoreWithConfig(crlURL, cfg.CRLRefreshInterval, cpHTTPClient, log)
	} else {
		crlStore = crl.NewStore()
	}

	// Cache de décisions d'autorisation
	decisionCache := decisioncache.New(cfg.DecisionCacheMaxKeys)

	// Heartbeat client
	var hbClient *heartbeat.Client
	if cpHTTPClient != nil {
		hbClient = heartbeat.NewClient(cfg, cpHTTPClient, log)
	}

	// Telemetry client
	var telemetryClient *telemetry.Client
	if cpHTTPClient != nil {
		telemetryClient = telemetry.NewClient(cfg, cpHTTPClient, log)
	}

	// Handler de protocole CONNECT
	connectHandler := protocol.NewHandler(authzClient, tcpProxy, sessionMgr, log)
	connectHandler.SetCRLStore(crlStore)
	connectHandler.SetDecisionCache(decisionCache, cfg.DecisionCacheTTL)
	connectHandler.SetCPDownMode(cfg.CPDownMode)
	connectHandler.SetConfig(cfg)
	if telemetryClient != nil {
		connectHandler.SetTelemetryClient(telemetryClient)
	}

	// Client de résolution de ressources publiées via le CP
	resClient := resourceclient.NewClient(cfg, log)
	connectHandler.SetResourceClient(resClient)

	// Câbler la révocation active : quand la CRL détecte de nouveaux serials
	// révoqués, fermer immédiatement les sessions concernées.
	crlStore.OnRevocationUpdate(func(newlyRevoked []string) {
		for _, serial := range newlyRevoked {
			killed := sessionMgr.KillBySerial(serial)
			if killed > 0 {
				log.Warn("sessions fermées suite à révocation CRL",
					"cert_serial", serial,
					"sessions_killed", killed,
				)
			}
		}
	})

	// Listener mTLS
	listener, err := mtls.NewListener(cfg, connectHandler, log)
	if err != nil {
		return nil, fmt.Errorf("impossible de créer le listener mTLS: %w", err)
	}
	// Wire CRL into listener for handshake-time revocation check
	listener.SetCRLStore(crlStore)

	return &App{
		cfg:       cfg,
		log:       log,
		listener:  listener,
		handler:   connectHandler,
		authz:     authzClient,
		proxy:     tcpProxy,
		sessions:  sessionMgr,
		crl:       crlStore,
		cache:     decisionCache,
		heartbeat: hbClient,
		telemetry: telemetryClient,
	}, nil
}

// Run démarre tous les composants de la Gateway.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("démarrage de la Gateway ZTNA",
		"addr", a.cfg.Server.ListenAddr,
		"gateway_id", a.cfg.GatewayID,
		"cp_down_mode", a.cfg.CPDownMode,
		"decision_cache_ttl", a.cfg.DecisionCacheTTL.String(),
		"crl_refresh_interval", a.cfg.CRLRefreshInterval.String(),
		"heartbeat_every", a.cfg.HeartbeatEvery.String(),
	)

	// 1. Démarrer le CRL auto-refresh (goroutine)
	go func() {
		if err := a.crl.StartAutoRefresh(ctx); err != nil {
			a.log.Error("CRL auto-refresh terminé avec erreur", "error", err)
		}
	}()

	// 2. Démarrer le garbage collector de sessions (goroutine)
	go a.sessions.StartGarbageCollector(ctx)

	// 3. Démarrer le heartbeat (goroutine)
	if a.heartbeat != nil {
		go func() {
			if err := a.heartbeat.StartLoop(ctx); err != nil {
				a.log.Error("heartbeat loop terminé avec erreur", "error", err)
			}
		}()
	}

	// 4. Démarrer le listener mTLS (bloquant)
	return a.listener.Listen(ctx)
}

// Close effectue l'arrêt graceful de la Gateway :
// 1. Ferme le listener (plus de nouvelles connexions)
// 2. Attend que les sessions actives se terminent (avec timeout du contexte)
// 3. Kill les sessions restantes si le deadline est dépassé
func (a *App) Close(ctx context.Context) error {
	active := a.sessions.ActiveCount()
	a.log.Info("arrêt graceful de la gateway",
		"active_sessions", active,
	)

	// 1. Fermer le listener (plus de nouvelles connexions)
	if err := a.listener.Close(); err != nil {
		a.log.Warn("erreur fermeture listener", "error", err)
	}

	// 2. Drain : attendre que les sessions actives se terminent
	if active > 0 {
		a.log.Info("drain des sessions actives en cours...", "count", active)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

	drainLoop:
		for {
			select {
			case <-ctx.Done():
				a.log.Warn("deadline d'arrêt atteint, kill des sessions restantes",
					"remaining", a.sessions.ActiveCount(),
				)
				break drainLoop
			case <-ticker.C:
				remaining := a.sessions.ActiveCount()
				if remaining == 0 {
					a.log.Info("toutes les sessions sont terminées")
					break drainLoop
				}
				a.log.Info("sessions en cours de drain", "remaining", remaining)
			}
		}
	}

	// 3. Kill forcé des sessions restantes
	for _, s := range a.sessions.ListActive() {
		a.sessions.KillSession(s.ID)
	}

	// 4. Vidage du cache de décisions
	a.cache.Clear()

	a.log.Info("gateway arrêtée proprement")
	return nil
}
