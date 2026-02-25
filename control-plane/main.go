package main
// point d'entrée de binaire de mon control plane , il orchestre le démarage et l'arrêt des service 
// qui sont délégéer aux internals
import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"control-plane/internal/app"
	"control-plane/internal/config"
	"control-plane/internal/logger"
)

func main() {
	// chargement de la 
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.Logging.Level, cfg.Logging.Format)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// donner la main au APP pour construire notre CP
	application, err := app.New(ctx, cfg, log)
	if err != nil {
		log.Error("app init failed", slog.Any("err", err))
		os.Exit(1)
	}

	// demarrage du serveur et gestion des erreurs avec goroutine
	go func() {
		if err := application.Run(ctx); err != nil {
			log.Error("server stopped", slog.Any("err", err))
			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = application.Close(shutdownCtx)
}
