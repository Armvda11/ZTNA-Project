package httpserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"control-plane/internal/api/handlers"
	"control-plane/internal/api/middleware"
	"control-plane/internal/config"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	publicServer *http.Server
	pepServer    *http.Server
	publicCert   string
	publicKey    string
	pepCert      string
	pepKey       string
}

type Dependencies struct {
	CredentialsHandler  *handlers.CredentialsHandler
	DeviceCertHandler   *handlers.DeviceCertHandler
	PKIHandler          *handlers.PKIHandler
	AdminDeviceCerts    *handlers.AdminDeviceCertsHandler
	PEPHandler          *handlers.PEPHandler
	PEPRegisterHandler  *handlers.PEPRegisterHandler
	PEPHeartbeatHandler *handlers.PEPHeartbeatHandler
	PEPSessionHandler   *handlers.PEPSessionHandler // télémétrie de session
	AdminSessionHandler *handlers.AdminSessionsHandler // kill session admin
	AdminPolicies       *handlers.AdminPoliciesHandler
	AdminAudit          *handlers.AdminAuditHandler
	WhoamiHandler       *handlers.WhoamiHandler
	OIDC                *middleware.OIDCValidator
	PEPAuth             *middleware.PEPAuth
	AdminAuth           *middleware.AdminAuth
	PublicRateLimiter   *middleware.RateLimiter
	PEPRateLimiter      *middleware.RateLimiter
}

func New(cfg *config.Config, deps Dependencies) (*Server, error) {
	publicRouter := chi.NewRouter()
	publicRouter.Use(chimw.Recoverer)
	publicRouter.Use(middleware.RequestID)
	if deps.PublicRateLimiter != nil {
		publicRouter.Use(deps.PublicRateLimiter.Handler)
	}

	publicRouter.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	publicRouter.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(deps.OIDC.RequireUser)
			r.Get("/whoami", deps.WhoamiHandler.Get)
		})

		r.Route("/credentials", func(r chi.Router) {
			r.Use(deps.OIDC.RequireUser)
			r.Post("/ssh-cert", deps.CredentialsHandler.IssueSSHCert)
			if deps.DeviceCertHandler != nil {
				r.Post("/device-cert", deps.DeviceCertHandler.Issue)
			}
		})

		// Token mode exposes PEP endpoints on the public API listener.
		if cfg.PEP.AuthMode != "mtls" {
			r.Route("/pep", func(r chi.Router) {
				r.Use(deps.PEPAuth.RequirePEP)
				if deps.PEPRegisterHandler != nil {
					r.Post("/register", deps.PEPRegisterHandler.Register)
				}
				r.Post("/authorize", deps.PEPHandler.Authorize)
				if deps.PEPHeartbeatHandler != nil {
					r.Post("/heartbeat", deps.PEPHeartbeatHandler.Beat)
				}
				if deps.PEPSessionHandler != nil {
					r.Post("/sessions/start", deps.PEPSessionHandler.Start)
					r.Post("/sessions/end", deps.PEPSessionHandler.End)
					r.Get("/sessions/{id}/valid", deps.PEPSessionHandler.Valid)
				}
			})
		}

		r.Route("/admin", func(r chi.Router) {
			r.Use(deps.OIDC.RequireUser)
			r.Use(deps.AdminAuth.RequireAdmin)
			r.Post("/policies", deps.AdminPolicies.CreateVersion)
			r.Post("/policies/{id}/activate", deps.AdminPolicies.ActivateVersion)
			r.Get("/policies/active", deps.AdminPolicies.ActivePolicy)
			r.Get("/audit", deps.AdminAudit.List)
			if deps.AdminDeviceCerts != nil {
				r.Delete("/device-certs/{serial}", deps.AdminDeviceCerts.Revoke)
			}
			if deps.PEPSessionHandler != nil {
				r.Get("/sessions", deps.PEPSessionHandler.List)
			}
			if deps.AdminSessionHandler != nil {
				r.Get("/sessions/{id}", deps.AdminSessionHandler.Get)
				r.Delete("/sessions/{id}", deps.AdminSessionHandler.Kill)
			}
		})
	})

	// PKI endpoints — no authentication, required for gateway bootstrap.
	if deps.PKIHandler != nil {
		publicRouter.Route("/pki", func(r chi.Router) {
			r.Get("/device-ca/cert", deps.PKIHandler.CACert)
			r.Get("/device-ca/crl", deps.PKIHandler.CRL)
			r.Get("/ssh-ca/pubkey", deps.PKIHandler.SSHCAPubKey)
		})
	}

	publicAddr := fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port)
	publicServer := &http.Server{
		Addr:              publicAddr,
		Handler:           publicRouter,
		ReadTimeout:       cfg.Server.ReadTimeoutDuration(),
		WriteTimeout:      cfg.Server.WriteTimeoutDuration(),
		IdleTimeout:       cfg.Server.IdleTimeoutDuration(),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeoutDuration(),
	}

	publicTLS, publicCert, publicKey, err := buildTLSConfig(cfg.Server.TLS, false)
	if err != nil {
		return nil, err
	}
	publicServer.TLSConfig = publicTLS

	var pepServer *http.Server
	pepCert := ""
	pepKey := ""
	// mTLS mode isolates PEP traffic on a dedicated listener/port.
	if cfg.PEP.AuthMode == "mtls" {
		pepRouter := chi.NewRouter()
		pepRouter.Use(chimw.Recoverer)
		pepRouter.Use(middleware.RequestID)
		if deps.PEPRateLimiter != nil {
			pepRouter.Use(deps.PEPRateLimiter.HandlerByPEP)
		}
		pepRouter.Route("/api/v1/pep", func(r chi.Router) {
			r.Use(deps.PEPAuth.RequirePEP)
			if deps.PEPRegisterHandler != nil {
				r.Post("/register", deps.PEPRegisterHandler.Register)
			}
			r.Post("/authorize", deps.PEPHandler.Authorize)
			if deps.PEPHeartbeatHandler != nil {
				r.Post("/heartbeat", deps.PEPHeartbeatHandler.Beat)
			}
			if deps.PEPSessionHandler != nil {
				r.Post("/sessions/start", deps.PEPSessionHandler.Start)
				r.Post("/sessions/end", deps.PEPSessionHandler.End)
				r.Get("/sessions/{id}/valid", deps.PEPSessionHandler.Valid)
			}
		})

		pepAddr := fmt.Sprintf("%s:%d", cfg.PEPServer.Address, cfg.PEPServer.Port)
		pepServer = &http.Server{
			Addr:              pepAddr,
			Handler:           pepRouter,
			ReadTimeout:       cfg.PEPServer.ReadTimeoutDuration(),
			WriteTimeout:      cfg.PEPServer.WriteTimeoutDuration(),
			IdleTimeout:       cfg.PEPServer.IdleTimeoutDuration(),
			ReadHeaderTimeout: cfg.PEPServer.ReadHeaderTimeoutDuration(),
		}

		pepTLS, certFile, keyFile, err := buildTLSConfig(cfg.PEPServer.TLS, true)
		if err != nil {
			return nil, err
		}
		pepServer.TLSConfig = pepTLS
		pepCert = certFile
		pepKey = keyFile
	}

	return &Server{
		publicServer: publicServer,
		pepServer:    pepServer,
		publicCert:   publicCert,
		publicKey:    publicKey,
		pepCert:      pepCert,
		pepKey:       pepKey,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 2)
	count := 0

	if s.publicServer != nil {
		count++
		go func() {
			errCh <- runServer(ctx, s.publicServer, s.publicCert, s.publicKey)
		}()
	}
	if s.pepServer != nil {
		count++
		go func() {
			errCh <- runServer(ctx, s.pepServer, s.pepCert, s.pepKey)
		}()
	}

	var firstErr error
	for i := 0; i < count; i++ {
		err := <-errCh
		if err != nil && firstErr == nil {
			firstErr = err
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = s.Close(shutdownCtx)
			cancel()
		}
	}

	return firstErr
}

func (s *Server) Close(ctx context.Context) error {
	var firstErr error
	if s.publicServer != nil {
		if err := s.publicServer.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.pepServer != nil {
		if err := s.pepServer.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func runServer(ctx context.Context, server *http.Server, certFile, keyFile string) error {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	var err error
	if server.TLSConfig != nil {
		err = server.ListenAndServeTLS(certFile, keyFile)
	} else {
		err = server.ListenAndServe()
	}
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func buildTLSConfig(tlsCfg config.TLSConfig, forceClientAuth bool) (*tls.Config, string, string, error) {
	if !tlsCfg.Enabled {
		return nil, "", "", nil
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}
	certFile := tlsCfg.CertFile
	keyFile := tlsCfg.KeyFile

	if forceClientAuth || tlsCfg.RequireClientAuth {
		caCert, err := os.ReadFile(tlsCfg.ClientCAFile)
		if err != nil {
			return nil, "", "", fmt.Errorf("read client ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, "", "", fmt.Errorf("parse client ca")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsConfig, certFile, keyFile, nil
}
