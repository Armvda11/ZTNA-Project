package app

import (
	"context"
	"fmt"
	"log/slog"

	"control-plane/internal/api/handlers"
	"control-plane/internal/api/httpserver"
	"control-plane/internal/api/middleware"
	"control-plane/internal/config"
	"control-plane/internal/crypto/sshca"
	"control-plane/internal/logger"
	"control-plane/internal/service/audit"
	"control-plane/internal/service/credentials"
	"control-plane/internal/service/decision"
	"control-plane/internal/service/policy"
	"control-plane/internal/store/sqlite"
)

type App struct {
	cfg    *config.Config
	log    *slog.Logger
	store  *sqlite.Store
	server *httpserver.Server
}

func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	store, err := sqlite.Open(cfg.Database.Path, cfg.BusyTimeout(), cfg.Database.Pragmas)
	if err != nil {
		return nil, err
	}

	ca, err := sshca.LoadOrCreate(cfg.SSHCA.KeyPath)
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	policySvc := policy.New(store)
	auditSvc := audit.New(store)
	credsSvc := credentials.New(ca, cfg.SSHCA, store)
	decisionSvc := decision.New(policySvc)
	if err := policySvc.SeedIfEmpty(ctx, cfg.Policy.SeedFile); err != nil {
		_ = store.Close()
		return nil, err
	}

	oidcValidator, err := middleware.NewOIDCValidator(cfg.OIDC)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("init oidc: %w", err)
	}

	pepAuth := middleware.NewPEPAuth(cfg.PEP)
	adminAuth := middleware.NewAdminAuth(cfg.OIDC)

	server, err := httpserver.New(cfg, httpserver.Dependencies{
		CredentialsHandler: handlers.NewCredentialsHandler(credsSvc, auditSvc),
		PEPHandler:         handlers.NewPEPHandler(decisionSvc, auditSvc),
		AdminPolicies:      handlers.NewAdminPoliciesHandler(policySvc, auditSvc),
		AdminAudit:         handlers.NewAdminAuditHandler(auditSvc),
		WhoamiHandler:      handlers.NewWhoamiHandler(),
		OIDC:               oidcValidator,
		PEPAuth:            pepAuth,
		AdminAuth:          adminAuth,
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
