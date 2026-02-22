// Package main est le point d'entrée de la Gateway ZTNA. Elle écoute
// les connexions mTLS entrantes depuis les clients ZTNA, vérifie
// l'identité du client (authn locale via certificat), demande une
// décision d'autorisation au Control Plane et relaie le trafic vers
// les ressources autorisées.
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

	"gateway/internal/app"
	"gateway/internal/config"
	"gateway/internal/logger"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "chemin vers le fichier de configuration")
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

	// Lancer l'application dans une goroutine
	go func() {
		if err := application.Run(ctx); err != nil {
			log.Error("erreur durant l'exécution", "error", err)
		}
	}()

	log.Info("ztna-gateway démarrée", "listen", cfg.Server.ListenAddr)

	// Attendre le signal d'arrêt
	<-ctx.Done()
	log.Info("arrêt en cours...")

	// Arrêt graceful avec timeout de 10 secondes
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := application.Close(shutdownCtx); err != nil {
		log.Error("erreur lors de l'arrêt", "error", err)
	}

	log.Info("ztna-gateway arrêtée")
}
