package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ztna-gateway/internal/config"
	"ztna-gateway/internal/pep"
	"ztna-gateway/internal/proxy"
	"ztna-gateway/internal/tlsutil"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to gateway config file")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("failed to load config", slog.Any("err", err))
		os.Exit(1)
	}

	cpHTTP := newHTTPClient(cfg.CPTLSInsecure)

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

	pepClient := pep.New(cfg.CPURL, cfg.PEPID, cfg.PEPToken, cpHTTP)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	heartbeatInterval := cfg.HeartbeatEvery
	if heartbeatInterval == 0 {
		heartbeatInterval = 30 * time.Second
	}
	go heartbeatLoop(ctx, pepClient, heartbeatInterval, log)

	srv := proxy.New(cfg, pepClient, log)
	if err := srv.ListenAndServe(ctx, tlsConfig); err != nil {
		log.Error("gateway exited with error", slog.Any("err", err))
		os.Exit(1)
	}

	log.Info("gateway stopped")
}

func newHTTPClient(insecure bool) *http.Client {
	if !insecure {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
}

func heartbeatLoop(ctx context.Context, client *pep.Client, interval time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Heartbeat(ctx); err != nil {
				log.Warn("heartbeat failed", slog.Any("err", err))
			} else {
				log.Debug("heartbeat ok")
			}
		}
	}
}
