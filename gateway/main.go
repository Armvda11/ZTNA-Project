package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ztna-gateway/internal/config"
	"ztna-gateway/internal/crl"
	"ztna-gateway/internal/pep"
	"ztna-gateway/internal/proxy"
	"ztna-gateway/internal/tlsutil"
)

var version = "dev"

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to gateway config file")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("failed to load config", slog.Any("err", err))
		os.Exit(1)
	}

	cpHTTP, err := newCPHTTPClient(cfg)
	if err != nil {
		log.Error("failed to build CP HTTP client", slog.Any("err", err))
		os.Exit(1)
	}

	// Startup sequence: resolve trust roots first, then open TLS listener.
	// Load or fetch the Device CA certificate to verify client certs.
	var caCertPEM []byte
	if cfg.TLS.DeviceCACert != "" {
		caCertPEM, err = tlsutil.LoadDeviceCACertFromFile(cfg.TLS.DeviceCACert)
		if err != nil {
			log.Error("failed to load device CA cert", slog.Any("err", err))
			os.Exit(1)
		}
	} else {
		log.Info("fetching Device CA cert from CP", slog.String("cp_url", cfg.CPURL))
		for attempt := 1; attempt <= 10; attempt++ {
			caCertPEM, err = tlsutil.FetchDeviceCACert(cfg.CPURL, cpHTTP)
			if err == nil {
				break
			}
			log.Warn("CP not ready, retrying", slog.Int("attempt", attempt), slog.Any("err", err))
			time.Sleep(time.Duration(attempt*3) * time.Second)
		}
		if err != nil {
			log.Error("could not fetch Device CA cert", slog.Any("err", err))
			os.Exit(1)
		}
	}

	clientCAs, err := tlsutil.BuildClientCertPool(caCertPEM)
	if err != nil {
		log.Error("build client cert pool", slog.Any("err", err))
		os.Exit(1)
	}

	// Load or generate an ephemeral server certificate.
	serverCert, err := tlsutil.LoadOrGenerateSelfSignedCert(cfg.TLS.ServerCert, cfg.TLS.ServerKey)
	if err != nil {
		log.Error("server cert", slog.Any("err", err))
		os.Exit(1)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    clientCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	pepClient := pep.New(cfg.CPURL, cfg.PEPID, cfg.PEPToken, cfg.CPAuthMode, cpHTTP)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// CRL cache : chargé immédiatement au démarrage, rafraîchissement périodique.
	// Si le CP est inaccessible au boot, on démarre avec une CRL vide (fail-open)
	// et on log un avertissement fort. En prod, durcir en fail-closed ici.
	crlCache := crl.New(log)
	srv := proxy.New(cfg, pepClient, crlCache, log)
	if err := crlCache.StartRefreshLoop(
		ctx,
		cfg.CRLRefreshInterval,
		cfg.CPURL,
		cpHTTP,
		func() { srv.KillRevoked(crlCache) },
		cfg.StrictRevocationEnabled(),
	); err != nil {
		log.Error("crl strict startup check failed", slog.Any("err", err))
		os.Exit(1)
	}

	fingerprint := certFingerprint(serverCert)
	if cfg.RequireRegistrationEnabled() {
		if err := registerGateway(ctx, pepClient, cfg, fingerprint, log); err != nil {
			log.Error("gateway registration failed", slog.Any("err", err))
			os.Exit(1)
		}
	}

	// Heartbeat is best-effort and should never block gateway traffic.
	heartbeatInterval := cfg.HeartbeatEvery
	if heartbeatInterval == 0 {
		heartbeatInterval = 30 * time.Second
	}
	go heartbeatLoop(ctx, pepClient, cfg, fingerprint, heartbeatInterval, log)

	if err := srv.ListenAndServe(ctx, tlsConfig); err != nil {
		log.Error("gateway exited with error", slog.Any("err", err))
		os.Exit(1)
	}

	log.Info("gateway stopped")
}

func newCPHTTPClient(cfg *config.Config) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
	if cfg.CPTLSInsecure {
		tlsCfg.InsecureSkipVerify = true //nolint:gosec
	} else if cfg.CPCACert != "" {
		pool, err := tlsutil.LoadCertPoolFromFile(cfg.CPCACert)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	} else {
		// If not pinned explicitly, still use system roots.
		pool, err := x509.SystemCertPool()
		if err == nil && pool != nil {
			tlsCfg.RootCAs = pool
		}
	}

	if cfg.CPAuthMode == "mtls" {
		cert, err := tls.LoadX509KeyPair(cfg.CPClientCert, cfg.CPClientKey)
		if err != nil {
			return nil, fmt.Errorf("load cp client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}, nil
}

func registerGateway(
	ctx context.Context,
	client *pep.Client,
	cfg *config.Config,
	fingerprint string,
	log *slog.Logger,
) error {
	var lastErr error
	for attempt := 1; attempt <= 10; attempt++ {
		err := client.Register(ctx, pep.RegisterRequest{
			GatewayID:   cfg.GatewayID,
			Name:        cfg.GatewayID,
			Version:     version,
			Fingerprint: fingerprint,
		})
		if err == nil {
			log.Info("gateway registered", slog.String("gateway_id", cfg.GatewayID))
			return nil
		}
		lastErr = err
		log.Warn("register failed, retrying", slog.Int("attempt", attempt), slog.Any("err", err))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	return lastErr
}

func heartbeatLoop(
	ctx context.Context,
	client *pep.Client,
	cfg *config.Config,
	fingerprint string,
	interval time.Duration,
	log *slog.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status, err := client.Heartbeat(ctx)
			if err != nil {
				var apiErr *pep.APIError
				if cfg.RequireRegistrationEnabled() && errors.As(err, &apiErr) && apiErr.IsStatus(http.StatusForbidden) {
					log.Warn("heartbeat rejected, attempting re-register", slog.String("status", apiErr.Status))
					if regErr := registerGateway(ctx, client, cfg, fingerprint, log); regErr != nil {
						log.Warn("re-register failed", slog.Any("err", regErr))
					}
				}
				log.Warn("heartbeat failed", slog.Any("err", err))
			} else {
				log.Debug("heartbeat ok", slog.String("status", status))
			}
		}
	}
}

func certFingerprint(cert tls.Certificate) string {
	if len(cert.Certificate) == 0 {
		return ""
	}
	return tlsutil.CertFingerprintSHA256(cert.Certificate[0])
}
