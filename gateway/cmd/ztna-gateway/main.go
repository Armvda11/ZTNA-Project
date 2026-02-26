package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	app "ztna-gateway/internal/bootstrap"
	"ztna-gateway/internal/config"
	"ztna-gateway/internal/observability/logger"
)

var version = "dev"

func main() {
	_ = version
	cfgPath := flag.String("config", "config.lab.yaml", "chemin vers le fichier de configuration")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erreur de configuration: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Logging.Level, cfg.Logging.Format)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, log)
	if err != nil {
		log.Error("impossible de créer l'application", "error", err)
		os.Exit(1)
	}

	go func() {
		if err := application.Run(ctx); err != nil {
			log.Error("erreur durant l'exécution", "error", err)
		}
	}()

	log.Info("ztna-gateway démarrée", "listen", cfg.Server.ListenAddr)

	<-ctx.Done()
	log.Info("arrêt en cours...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := application.Close(shutdownCtx); err != nil {
		log.Error("erreur lors de l'arrêt", "error", err)
	}

	log.Info("ztna-gateway arrêtée")
}
