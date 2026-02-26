// Package oidc gère l'authentification OpenID Connect pour le client ZTNA.
// Il fournit les mécanismes d'obtention et de rafraîchissement des tokens
// d'accès auprès du fournisseur OIDC (Keycloak).
//
// Deux modes d'authentification sont prévus :
//   - Resource Owner Password Grant (lab uniquement, dangereux en production)
//   - Device Authorization Flow (recommandé pour les utilisateurs finaux)
//
// Le client OIDC ne valide PAS les tokens localement ; c'est le rôle
// du Control Plane. Le client se contente d'obtenir et de stocker les tokens.
package oidc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"client/internal/config"
)

// TokenSet contient les tokens OIDC obtenus après authentification.
type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Client est le client OIDC du ZTNA. Il encapsule la configuration
// et le logger pour effectuer les flux d'authentification.
type Client struct {
	cfg   *config.Config
	log   *slog.Logger
	store *TokenStore
}

// NewClient crée un nouveau client OIDC configuré.
func NewClient(cfg *config.Config, log *slog.Logger) *Client {
	return &Client{
		cfg:   cfg,
		log:   log,
		store: NewTokenStore(cfg.Storage.Path, log),
	}
}

// LoginPasswordGrantLAB effectue un flux Resource Owner Password Credentials
// pour obtenir un access_token. Ce flux est DANGEREUX et ne doit être utilisé
// qu'en environnement de lab contrôlé.
//
// ⚠️  AVERTISSEMENT SÉCURITÉ :
//   - Le mot de passe transite en clair vers le provider OIDC
//   - Ce flux est déprécié par OAuth 2.1
//   - En production, utiliser DeviceFlowLogin() ou Authorization Code + PKCE
//
// TODO: Implémenter l'appel HTTP POST vers l'endpoint token du provider OIDC :
//
//	POST {issuer}/protocol/openid-connect/token
//	Content-Type: application/x-www-form-urlencoded
//
//	grant_type=password
//	&client_id={client_id}
//	&client_secret={client_secret}   (si configuré)
//	&username={username}
//	&password={password}
//	&scope=openid profile
//
// TODO: Parser la réponse JSON en TokenSet
// TODO: Stocker le TokenSet via a.store.Save()
func (c *Client) LoginPasswordGrantLAB(ctx context.Context, username, password string) (*TokenSet, error) {
	c.log.Warn("utilisation du flux Resource Owner Password Grant (lab uniquement)")

	// TODO: construire la requête HTTP vers {issuer}/protocol/openid-connect/token
	// TODO: envoyer la requête avec les paramètres grant_type=password
	// TODO: parser la réponse JSON
	// TODO: stocker les tokens

	return nil, fmt.Errorf("TODO: LoginPasswordGrantLAB non implémenté")
}

// DeviceFlowLogin effectue un flux Device Authorization pour permettre
// à l'utilisateur de s'authentifier via un navigateur externe.
//
// Ce flux est le mode recommandé pour les clients CLI :
//  1. Le client demande un device_code et un user_code au provider
//  2. L'utilisateur ouvre l'URL affichée et entre le user_code
//  3. Le client poll le provider jusqu'à obtenir les tokens
//
// TODO: Implémenter le flux Device Authorization :
//   - POST {issuer}/protocol/openid-connect/auth/device pour obtenir device_code
//   - Afficher l'URL de vérification et le user_code à l'utilisateur
//   - Poll {issuer}/protocol/openid-connect/token avec grant_type=urn:ietf:params:oauth:grant-type:device_code
//   - Gérer les réponses authorization_pending, slow_down, expired_token
func (c *Client) DeviceFlowLogin(ctx context.Context) (*TokenSet, error) {
	c.log.Info("démarrage du flux Device Authorization")

	// TODO: implémenter le flux Device Authorization complet

	return nil, fmt.Errorf("TODO: DeviceFlowLogin non implémenté")
}

// RefreshAccessToken utilise le refresh_token stocké pour obtenir un
// nouveau access_token sans ré-authentification de l'utilisateur.
//
// TODO: Implémenter l'appel HTTP :
//
//	POST {issuer}/protocol/openid-connect/token
//	grant_type=refresh_token
//	&client_id={client_id}
//	&refresh_token={refresh_token}
//
// TODO: Mettre à jour le TokenSet stocké
func (c *Client) RefreshAccessToken(ctx context.Context) (*TokenSet, error) {
	c.log.Info("rafraîchissement de l'access token")

	// TODO: charger le refresh_token depuis le store
	// TODO: appeler l'endpoint token avec grant_type=refresh_token
	// TODO: mettre à jour le store avec les nouveaux tokens

	return nil, fmt.Errorf("TODO: RefreshAccessToken non implémenté")
}

// GetValidAccessToken retourne un access_token valide, en le rafraîchissant
// si nécessaire. C'est la méthode principale à utiliser par les autres
// composants (credentials, tunnel) pour obtenir un token.
//
// TODO: Vérifier l'expiration du token stocké (avec marge de sécurité)
// TODO: Si expiré, appeler RefreshAccessToken()
// TODO: Si le refresh échoue, retourner une erreur indiquant de relancer login
func (c *Client) GetValidAccessToken(ctx context.Context) (string, error) {
	// TODO: charger le TokenSet depuis le store
	// TODO: vérifier si le token est encore valide (expires_at - margin)
	// TODO: si expiré, tenter un refresh
	// TODO: retourner l'access_token

	return "", fmt.Errorf("TODO: GetValidAccessToken non implémenté")
}
