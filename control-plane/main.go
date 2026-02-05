package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ztna/control-plane/internal/api"
	"github.com/ztna/control-plane/internal/config"
	"github.com/ztna/control-plane/internal/logger"
	"github.com/ztna/control-plane/internal/sshca"
	"github.com/ztna/control-plane/internal/storage"
)

const (
	defaultConfigPath = "/etc/ztna/config.yaml"
	version           = "0.1.0"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", defaultConfigPath, "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ZTNA Control Plane v%s\n", version)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	logger := logger.New(cfg.Logging)
	logger.Info("Starting ZTNA Control Plane", "version", version)

	// Initialize SSH CA
	ca, err := sshca.New(cfg.SSH, logger)
	if err != nil {
		logger.Error("Failed to initialize SSH CA", "error", err)
		os.Exit(1)
	}
	logger.Info("SSH CA initialized", "ca_fingerprint", ca.Fingerprint())

	// Initialize storage
	store, err := storage.New(cfg.Database, logger)
	if err != nil {
		logger.Error("Failed to initialize storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	logger.Info("Storage initialized", "type", cfg.Database.Type)

	// Initialize API server
	apiServer := api.NewServer(cfg, ca, store, logger)

	// Configure HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      apiServer.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		if cfg.Server.TLS.Enabled {
			logger.Info("Starting HTTPS server", "address", addr)
			if err := srv.ListenAndServeTLS(cfg.Server.TLS.Cert, cfg.Server.TLS.Key); err != nil && err != http.ErrServerClosed {
				logger.Error("Server failed", "error", err)
				os.Exit(1)
			}
			return
		}

		logger.Info("Starting HTTP server", "address", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("ZTNA Control Plane is ready", "address", addr)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown with 30 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("Server exited gracefully")
}
