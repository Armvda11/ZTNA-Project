// Package app fournit le câblage principal de l'application client ZTNA.
// Il assemble la configuration, le logger, le client OIDC, le gestionnaire
// de certificats et le tunnel mTLS. Les méthodes RunLogin, RunCert et
// RunConnect correspondent aux sous-commandes CLI.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"client/internal/config"
	"client/internal/infra/credentials"
	"client/internal/infra/oidc"
	"client/internal/infra/storage"
	"client/internal/infra/tunnel"
	"client/internal/usecase/connect"
	"client/internal/usecase/issuecert"
	"client/internal/usecase/login"
)

// App regroupe tous les composants nécessaires à l'exécution du client ZTNA.
type App struct {
	cfg    *config.Config
	log    *slog.Logger
	oidc   *oidc.Client
	creds  *credentials.Client
	tunnel *tunnel.Manager

	loginUC   *login.Service
	certUC    *issuecert.Service
	connectUC *connect.Service
}

// New construit une instance App à partir de la configuration et du logger.
// Chaque composant est initialisé mais aucune connexion réseau n'est établie.
func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	oidcClient := oidc.NewClient(cfg, log)
	credsClient := credentials.NewClient(cfg, log)
	tunnelMgr := tunnel.NewManager(cfg, log)
	resourceCatalog := storage.NewResourceCatalogFile(cfg, log)

	loginUC := login.NewService(oidc.NewAdapter(oidcClient), log)
	certUC := issuecert.NewService(oidcClient, credsClient, log)
	connectUC := connect.NewService(resourceCatalog, credsClient, tunnelMgr, log)

	return &App{
		cfg:    cfg,
		log:    log,
		oidc:   oidcClient,
		creds:  credsClient,
		tunnel: tunnelMgr,

		loginUC:   loginUC,
		certUC:    certUC,
		connectUC: connectUC,
	}, nil
}

// RunLogin effectue le flux d'authentification OIDC.
//
// Flux prévu :
//  1. Lancer le flux OIDC approprié :
//     - Lab : Resource Owner Password Grant (dangereux, lab uniquement)
//     - Production : Device Authorization Flow ou Authorization Code + PKCE
//  2. Recevoir l'access_token + refresh_token
//  3. Stocker les tokens de manière sécurisée (token_store)
//  4. Afficher un résumé (sub, username, expiration)
//
// TODO: supporter plusieurs providers OIDC si nécessaire
func (a *App) RunLogin(ctx context.Context) error {
	a.log.Info("démarrage du flux de login OIDC")

	// TODO: exposer des flags CLI pour choisir explicitement le mode de login
	// et passer username/password en mode lab.
	result, err := a.loginUC.Run(ctx, login.Options{Mode: login.ModeAuto})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("flux login OIDC interrompu: %w", err)
		}
		return fmt.Errorf("échec du flux login OIDC: %w", err)
	}

	if result == nil {
		return fmt.Errorf("échec du flux login OIDC: résultat vide")
	}
	if result.AccessToken == "" {
		return fmt.Errorf("échec du flux login OIDC: access_token manquant")
	}

	sub := result.Subject.Sub
	if sub == "" {
		sub = "n/a"
	}
	username := result.Subject.Username
	if username == "" {
		username = "n/a"
	}

	expiresAt := "inconnu"
	if !result.ExpiresAt.IsZero() {
		expiresAt = result.ExpiresAt.UTC().Format(time.RFC3339)
	}

	a.log.Info("login OIDC réussi",
		"sub", sub,
		"username", username,
		"expires_at", expiresAt,
		"refresh_token", result.RefreshToken != "",
	)
	return nil
}

// RunCert demande un certificat mTLS client au Control Plane.
//
// Flux prévu :
//  1. Charger l'access_token depuis le token_store
//  2. Générer une paire de clés (ECDSA P-256) localement
//  3. Construire un CSR (Certificate Signing Request)
//  4. Appeler POST /api/v1/credentials/mtls-cert sur le Control Plane
//     avec l'access_token en header Authorization: Bearer <token>
//  5. Recevoir le certificat signé (PEM)
//  6. Sauvegarder cert + clé privée dans storage.path
//
// IMPORTANT: L'endpoint CP /api/v1/credentials/mtls-cert n'existe pas encore.
//
//	Il sera implémenté ultérieurement dans le Control Plane.
//	Le client ne doit JAMAIS envoyer sa clé privée au CP.
func (a *App) RunCert(ctx context.Context) error {
	a.log.Info("demande de certificat mTLS au Control Plane")

	if err := a.certUC.Run(ctx); err != nil {
		return err
	}

	a.log.Info("certificat mTLS obtenu et sauvegardé avec succès")
	return nil
}

// RunConnect établit un tunnel mTLS vers la Gateway pour accéder à une
// ressource spécifique.
//
// Flux prévu :
//  1. Charger le certificat mTLS client depuis storage.path
//  2. Construire la tls.Config avec le certificat client et la CA de confiance
//  3. Établir une connexion TLS vers la Gateway (gateway.address)
//  4. Envoyer une requête CONNECT avec :
//     - action: "connect"
//     - resource: { type, host, port } (dérivé de resourceName)
//     - context: { src_ip, device_info, timestamp }
//  5. Attendre la réponse de la Gateway (allow/deny)
//  6. Si allow : relayer le trafic bidirectionnel (stdin/stdout ou port local)
//  7. Si deny : afficher le motif et quitter
//
// TODO: supporter le mode "port forwarding local" (écouter sur localhost:PORT)
func (a *App) RunConnect(ctx context.Context, resourceName string) error {
	a.log.Info("connexion à la ressource via tunnel mTLS", "resource", resourceName)

	if err := a.connectUC.Run(ctx, resourceName); err != nil {
		return err
	}

	a.log.Info("tunnel terminé proprement", "resource", resourceName)
	return nil
}
