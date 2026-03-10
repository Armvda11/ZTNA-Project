package app

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/time/rate"

	"control-plane/internal/api/handlers"
	"control-plane/internal/api/httpserver"
	"control-plane/internal/api/middleware"
	"control-plane/internal/config"
	"control-plane/internal/crypto/deviceca"
	"control-plane/internal/crypto/sshca"
	"control-plane/internal/logger"
	"control-plane/internal/service/audit"
	"control-plane/internal/service/credentials"
	"control-plane/internal/service/decision"
	"control-plane/internal/service/gateway"
	"control-plane/internal/service/policy"
	"control-plane/internal/service/resource"
	"control-plane/internal/service/session"
	"control-plane/internal/store/sqlite"
)

type App struct {
	cfg    *config.Config
	log    *slog.Logger
	store  *sqlite.Store
	server *httpserver.Server
}

func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	// Bootstrap order matters: storage and CAs must be ready before services/handlers.
	store, err := sqlite.Open(cfg.Database.Path, cfg.BusyTimeout(), cfg.Database.Pragmas)
	if err != nil {
		return nil, err
	}

	ca, err := sshca.LoadOrCreate(cfg.SSHCA.KeyPath)
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	deviceCA, err := deviceca.LoadOrCreate(cfg.DeviceCA.KeyPath, cfg.DeviceCA.CertPath)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("init device ca: %w", err)
	}

	// Build domain services once and inject them into handlers.
	policySvc := policy.New(store)
	auditSvc := audit.New(store)
	credsSvc := credentials.New(ca, cfg.SSHCA, store)
	deviceCredsSvc := credentials.NewDeviceCertService(deviceCA, cfg.DeviceCA, store)
	gatewaySvc := gateway.New(store)
	decisionSvc := decision.New(policySvc, cfg.PEP.DecisionTTLSeconds)
	sessionSvc := session.New(store)
	resourceSvc := resource.New(store)
	// Seed policy is idempotent and only runs on empty DB.
	if err := policySvc.SeedIfEmpty(ctx, cfg.Policy.SeedFile); err != nil {
		_ = store.Close()
		return nil, err
	}
	// Seed resources is idempotent and only runs on empty table.
	if err := resourceSvc.SeedIfEmpty(ctx, cfg.Resource.SeedFile); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("seed resources: %w", err)
	}

	oidcValidator, err := middleware.NewOIDCValidator(cfg.OIDC)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("init oidc: %w", err)
	}

	pepAuth := middleware.NewPEPAuth(cfg.PEP)
	adminAuth := middleware.NewAdminAuth(cfg.OIDC)

	// Rate limiters: 20 req/s (burst 40) pour le public, 100 req/s (burst 200) pour PEP.
	publicRL := middleware.NewIPRateLimiter(rate.Limit(20), 40)
	pepRL := middleware.NewIPRateLimiter(rate.Limit(100), 200)

	server, err := httpserver.New(cfg, httpserver.Dependencies{
		CredentialsHandler:  handlers.NewCredentialsHandler(credsSvc, auditSvc),
		DeviceCertHandler:   handlers.NewDeviceCertHandler(deviceCredsSvc, auditSvc),
		PKIHandler:          handlers.NewPKIHandler(deviceCredsSvc, credsSvc),
		AdminDeviceCerts:    handlers.NewAdminDeviceCertsHandler(deviceCredsSvc, auditSvc),
		PEPHandler:          handlers.NewPEPHandler(decisionSvc, auditSvc, gatewaySvc, cfg.PEPRequireRegistrationEnabled()),
		PEPRegisterHandler:  handlers.NewPEPRegisterHandler(gatewaySvc),
		PEPHeartbeatHandler: handlers.NewPEPHeartbeatHandler(gatewaySvc, cfg.PEPRequireRegistrationEnabled()),
		PEPSessionHandler:   handlers.NewPEPSessionHandler(sessionSvc),
		AdminPolicies:       handlers.NewAdminPoliciesHandler(policySvc, auditSvc),
		AdminAudit:          handlers.NewAdminAuditHandler(auditSvc),
		WhoamiHandler:       handlers.NewWhoamiHandler(),
		ResourceHandler:     handlers.NewResourceHandler(resourceSvc),
		PEPResourceHandler:  handlers.NewPEPResourceHandler(resourceSvc),
		OIDC:                oidcValidator,
		PEPAuth:             pepAuth,
		AdminAuth:           adminAuth,
		PublicRateLimiter:   publicRL,
		PEPRateLimiter:      pepRL,
	})
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	log.Info("app ready")

	return &App{cfg: cfg, log: log, store: store, server: server}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.log.Info("starting public server", slog.String("addr", fmt.Sprintf("%s:%d", a.cfg.Server.Address, a.cfg.Server.Port)))
	if a.cfg.PEP.AuthMode == "mtls" {
		a.log.Info("starting pep server", slog.String("addr", fmt.Sprintf("%s:%d", a.cfg.PEPServer.Address, a.cfg.PEPServer.Port)))
	}
	if err := a.server.Run(ctx); err != nil {
		return err
	}
	return nil
}

func (a *App) Close(ctx context.Context) error {
	if err := a.server.Close(ctx); err != nil {
		a.log.Error("http server shutdown", slog.Any("err", err))
	}
	if err := a.store.Close(); err != nil {
		a.log.Error("db close", slog.Any("err", err))
	}
	return nil
}

func RequestIDFromContext(ctx context.Context) string {
	return logger.RequestIDFromContext(ctx)
}
