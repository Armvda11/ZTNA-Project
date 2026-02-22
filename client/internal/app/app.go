// Package app fournit le câblage principal de l'application client ZTNA.
// Il assemble la configuration, le logger, le client OIDC, le gestionnaire
// de certificats et le tunnel mTLS. Les méthodes RunLogin, RunCert et
// RunConnect correspondent aux sous-commandes CLI.
package app

import (
	"context"
	"fmt"
	"log/slog"

	"client/internal/config"
	"client/internal/credentials"
	"client/internal/oidc"
	"client/internal/tunnel"
)

// App regroupe tous les composants nécessaires à l'exécution du client ZTNA.
type App struct {
	cfg    *config.Config
	log    *slog.Logger
	oidc   *oidc.Client
	creds  *credentials.Client
	tunnel *tunnel.Manager
}

// New construit une instance App à partir de la configuration et du logger.
// Chaque composant est initialisé mais aucune connexion réseau n'est établie.
func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	oidcClient := oidc.NewClient(cfg, log)
	credsClient := credentials.NewClient(cfg, log)
	tunnelMgr := tunnel.NewManager(cfg, log)

	return &App{
		cfg:    cfg,
		log:    log,
		oidc:   oidcClient,
		creds:  credsClient,
		tunnel: tunnelMgr,
	}, nil
}

// RunLogin effectue le flux d'authentification OIDC.
//
// Flux prévu (Method 3) :
//  1. Lancer le flux OIDC approprié :
//     - Lab : Resource Owner Password Grant (dangereux, lab uniquement)
//     - Production : Device Authorization Flow ou Authorization Code + PKCE
//  2. Recevoir l'access_token + refresh_token
//  3. Stocker les tokens de manière sécurisée (token_store)
//  4. Afficher un résumé (sub, username, expiration)
//
// TODO: implémenter le flux OIDC complet avec gestion d'erreurs
// TODO: supporter plusieurs providers OIDC si nécessaire
func (a *App) RunLogin(ctx context.Context) error {
	a.log.Info("démarrage du flux de login OIDC")

	// TODO: appeler a.oidc.LoginPasswordGrantLAB() ou a.oidc.DeviceFlowLogin()
	// TODO: stocker les tokens via a.oidc.Store()
	// TODO: afficher les informations du token (sub, exp, etc.)

	return fmt.Errorf("TODO: RunLogin non implémenté")
}

// RunCert demande un certificat mTLS client au Control Plane.
//
// Flux prévu (Method 3) :
//  1. Charger l'access_token depuis le token_store
//  2. Générer une paire de clés (ECDSA P-256) localement
//  3. Construire un CSR (Certificate Signing Request)
//  4. Appeler POST /api/v1/credentials/mtls-cert sur le Control Plane
//     avec l'access_token en header Authorization: Bearer <token>
//  5. Recevoir le certificat signé (PEM)
//  6. Sauvegarder cert + clé privée dans storage.path
//
// IMPORTANT: L'endpoint CP /api/v1/credentials/mtls-cert n'existe pas encore.
//            Il sera implémenté ultérieurement dans le Control Plane.
//            Le client ne doit JAMAIS envoyer sa clé privée au CP.
//
// TODO: implémenter la génération de clés et l'appel HTTP
func (a *App) RunCert(ctx context.Context) error {
	a.log.Info("demande de certificat mTLS au Control Plane")

	// TODO: vérifier qu'un access_token valide est disponible
	// TODO: appeler a.creds.RequestMTLSCertFromCP(accessToken)
	// TODO: sauvegarder le certificat et la clé privée

	return fmt.Errorf("TODO: RunCert non implémenté")
}

// RunConnect établit un tunnel mTLS vers la Gateway pour accéder à une
// ressource spécifique.
//
// Flux prévu (Method 3) :
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
// TODO: implémenter le tunnel complet avec gestion de session
// TODO: supporter le mode "port forwarding local" (écouter sur localhost:PORT)
func (a *App) RunConnect(ctx context.Context, resourceName string) error {
	a.log.Info("connexion à la ressource via tunnel mTLS", "resource", resourceName)

	// TODO: charger le certificat mTLS client
	// TODO: établir la connexion TLS vers la Gateway
	// TODO: envoyer la requête CONNECT (protocol.ConnectRequest)
	// TODO: vérifier la réponse (allow/deny)
	// TODO: relayer le trafic bidirectionnel

	return fmt.Errorf("TODO: RunConnect non implémenté")
}
