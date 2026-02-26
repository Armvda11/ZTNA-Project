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
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"client/internal/config"
	tlsinfra "client/internal/infra/tls"
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
	http  *http.Client
}

// NewClient crée un nouveau client OIDC configuré.
func NewClient(cfg *config.Config, log *slog.Logger) *Client {
	httpClient, err := tlsinfra.NewControlPlaneHTTPClient(cfg)
	if err != nil {
		log.Warn("client HTTP TLS par défaut indisponible, fallback sur client HTTP standard", "error", err)
		httpClient = http.DefaultClient
	}

	return &Client{
		cfg:   cfg,
		log:   log,
		store: NewTokenStore(cfg.Storage.Path, log),
		http:  httpClient,
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
func (c *Client) LoginPasswordGrantLAB(ctx context.Context, username, password string) (*TokenSet, error) {
	c.log.Warn("utilisation du flux Resource Owner Password Grant (lab uniquement)")

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", c.cfg.OIDC.ClientID)
	form.Set("username", username)
	form.Set("password", password)
	form.Set("scope", "openid profile")
	if c.cfg.OIDC.ClientSecret != "" {
		form.Set("client_secret", c.cfg.OIDC.ClientSecret)
	}

	tokenEndpoint := strings.TrimRight(c.cfg.OIDC.Issuer, "/") + "/protocol/openid-connect/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("impossible de construire la requête token OIDC: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("échec de l'appel à l'endpoint token OIDC: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("échec OIDC token endpoint (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("impossible de parser la réponse token OIDC: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("réponse token OIDC invalide: access_token manquant")
	}

	tokens := &TokenSet{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	if err := c.store.Save(tokens); err != nil {
		return nil, fmt.Errorf("impossible de stocker les tokens OIDC: %w", err)
	}

	return tokens, nil
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
	c.log.Info("dÃ©marrage du flux Device Authorization")

	deviceEndpoint := strings.TrimRight(c.cfg.OIDC.Issuer, "/") + "/protocol/openid-connect/auth/device"

	deviceForm := url.Values{}
	deviceForm.Set("client_id", c.cfg.OIDC.ClientID)
	deviceForm.Set("scope", "openid profile")
	if c.cfg.OIDC.ClientSecret != "" {
		deviceForm.Set("client_secret", c.cfg.OIDC.ClientSecret)
	}

	deviceReq, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceEndpoint, strings.NewReader(deviceForm.Encode()))
	if err != nil {
		return nil, fmt.Errorf("impossible de construire la requête device authorization: %w", err)
	}
	deviceReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	deviceResp, err := c.http.Do(deviceReq)
	if err != nil {
		return nil, fmt.Errorf("échec de l'appel device authorization OIDC: %w", err)
	}
	defer deviceResp.Body.Close()

	if deviceResp.StatusCode < http.StatusOK || deviceResp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(deviceResp.Body)
		return nil, fmt.Errorf("échec device authorization endpoint (%d): %s", deviceResp.StatusCode, strings.TrimSpace(string(body)))
	}

	var deviceCodeResp struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int64  `json:"expires_in"`
		Interval                int64  `json:"interval"`
	}
	if err := json.NewDecoder(deviceResp.Body).Decode(&deviceCodeResp); err != nil {
		return nil, fmt.Errorf("impossible de parser la réponse device authorization: %w", err)
	}
	if deviceCodeResp.DeviceCode == "" || deviceCodeResp.UserCode == "" {
		return nil, fmt.Errorf("réponse device authorization invalide: device_code/user_code manquant")
	}

	verificationURL := deviceCodeResp.VerificationURIComplete
	if verificationURL == "" {
		verificationURL = deviceCodeResp.VerificationURI
	}

	c.log.Info("validez la connexion dans le navigateur", "verification_url", verificationURL, "user_code", deviceCodeResp.UserCode)

	pollInterval := time.Duration(deviceCodeResp.Interval) * time.Second
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	expiresIn := time.Duration(deviceCodeResp.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 10 * time.Minute
	}
	deadline := time.Now().Add(expiresIn)

	tokenEndpoint := strings.TrimRight(c.cfg.OIDC.Issuer, "/") + "/protocol/openid-connect/token"

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("device_code expiré avant autorisation utilisateur")
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}

		pollForm := url.Values{}
		pollForm.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		pollForm.Set("client_id", c.cfg.OIDC.ClientID)
		pollForm.Set("device_code", deviceCodeResp.DeviceCode)
		if c.cfg.OIDC.ClientSecret != "" {
			pollForm.Set("client_secret", c.cfg.OIDC.ClientSecret)
		}

		pollReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(pollForm.Encode()))
		if err != nil {
			return nil, fmt.Errorf("impossible de construire la requête de polling token: %w", err)
		}
		pollReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		pollResp, err := c.http.Do(pollReq)
		if err != nil {
			return nil, fmt.Errorf("échec polling token endpoint OIDC: %w", err)
		}

		var tokenResp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			IDToken      string `json:"id_token"`
			TokenType    string `json:"token_type"`
			ExpiresIn    int64  `json:"expires_in"`
			Error        string `json:"error"`
		}

		decodeErr := json.NewDecoder(pollResp.Body).Decode(&tokenResp)
		pollResp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("impossible de parser la réponse de polling OIDC: %w", decodeErr)
		}

		if pollResp.StatusCode >= http.StatusOK && pollResp.StatusCode < http.StatusMultipleChoices {
			if tokenResp.AccessToken == "" {
				return nil, fmt.Errorf("réponse token OIDC invalide: access_token manquant")
			}

			tokens := &TokenSet{
				AccessToken:  tokenResp.AccessToken,
				RefreshToken: tokenResp.RefreshToken,
				IDToken:      tokenResp.IDToken,
				TokenType:    tokenResp.TokenType,
				ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
			}

			if err := c.store.Save(tokens); err != nil {
				return nil, fmt.Errorf("impossible de stocker les tokens OIDC: %w", err)
			}

			return tokens, nil
		}

		switch tokenResp.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			pollInterval += 2 * time.Second
			continue
		case "expired_token":
			return nil, fmt.Errorf("device_code expiré, relancez ztna login")
		default:
			return nil, fmt.Errorf("échec device flow token endpoint (%d): %s", pollResp.StatusCode, tokenResp.Error)
		}
	}
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
	tokens, err := c.store.Load()
	if err != nil {
		return "", fmt.Errorf("impossible de charger les tokens stockés: %w", err)
	}

	if tokens.AccessToken == "" {
		return "", fmt.Errorf("token stocké invalide: access_token manquant")
	}

	margin := config.TokenExpiryMargin()
	if tokens.ExpiresAt.IsZero() || time.Now().Before(tokens.ExpiresAt.Add(-margin)) {
		return tokens.AccessToken, nil
	}

	if tokens.RefreshToken == "" {
		return "", fmt.Errorf("access_token expiré et refresh_token absent: relancez 'ztna login'")
	}

	refreshed, refreshErr := c.RefreshAccessToken(ctx)
	if refreshErr != nil {
		return "", fmt.Errorf("access_token expiré et rafraîchissement impossible: %w", refreshErr)
	}
	if refreshed == nil || refreshed.AccessToken == "" {
		return "", fmt.Errorf("rafraîchissement token invalide: access_token manquant")
	}

	return refreshed.AccessToken, nil
}
