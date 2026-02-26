// Package main est le point d'entrée du client ZTNA. Il fournit une
// interface en ligne de commande avec les sous-commandes suivantes :
//   - ztna login     : authentification OIDC auprès de Keycloak
//   - ztna cert      : demande d'un certificat mTLS client au Control Plane
//   - ztna connect   : établissement d'un tunnel mTLS vers la Gateway
//
// Aucune logique métier ne réside ici ; tout est délégué à internal/app.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"client/internal/bootstrap"
	"client/internal/config"
	"client/internal/observability/logger"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "chemin vers le fichier de configuration")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

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

	subcmd := args[0]
	switch subcmd {
	case "login":
		if err := application.RunLogin(ctx); err != nil {
			log.Error("login échoué", "error", err)
			os.Exit(1)
		}
	case "cert":
		if err := application.RunCert(ctx); err != nil {
			log.Error("demande de certificat échouée", "error", err)
			os.Exit(1)
		}
	case "connect":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: ztna connect <resource> [--local-port <port>]\n")
			os.Exit(1)
		}
		resource := args[1]
		// Parser les flags spécifiques à connect après la ressource
		connectFlags := flag.NewFlagSet("connect", flag.ExitOnError)
		localPort := connectFlags.Int("local-port", 0, "port TCP local pour le mode port-forward")
		connectFlags.Parse(args[2:]) //nolint:errcheck
		if *localPort > 0 {
			if err := application.RunConnectPortForward(ctx, resource, *localPort); err != nil {
				log.Error("port-forward échoué", "error", err)
				os.Exit(1)
			}
		} else {
			if err := application.RunConnect(ctx, resource); err != nil {
				log.Error("connexion échouée", "error", err)
				os.Exit(1)
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "sous-commande inconnue: %s\n", subcmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: ztna [options] <command> [args]

Commands:
  login                      Authentification OIDC (Keycloak)
  cert                       Demander un certificat mTLS client au Control Plane
  connect <resource>         Tunnel mTLS stdin/stdout (mode ProxyCommand SSH)
  connect <resource>         Flags optionnels :
    --local-port <N>           Port-forward local : écoute sur 127.0.0.1:N

Exemples :
  ztna connect ssh:lan-app:22                          # ProxyCommand : relay stdin/stdout
  ztna connect http:lan-app:80 --local-port 18080      # port-forward + curl

Options:
  -config string     Chemin vers le fichier de configuration (défaut: config.yaml)
`)
}
