package oidc

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"client/internal/config"
)

// TestRefreshAccessToken_Success vérifie le flux complet de rafraîchissement :
// 1. Token expiré stocké avec refresh_token
// 2. Appel POST avec grant_type=refresh_token
// 3. Nouveau access_token + refresh_token stockés
func TestRefreshAccessToken_Success(t *testing.T) {
	tempDir := t.TempDir()

	var capturedForm url.Values
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path != "/realms/ztna/protocol/openid-connect/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		capturedForm, _ = url.ParseQuery(string(body))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "new-acc-789",
			"refresh_token": "new-ref-101",
			"token_type": "Bearer",
			"expires_in": 300
		}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   ts.URL + "/realms/ztna",
			ClientID: "ztna-client",
		},
		Storage: config.StorageConfig{Path: tempDir},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	// Stocker un token expiré avec refresh_token
	err := client.store.Save(&TokenSet{
		AccessToken:  "old-acc-expired",
		RefreshToken: "old-ref-456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-5 * time.Minute), // expiré
	})
	if err != nil {
		t.Fatalf("save initial tokens: %v", err)
	}

	// Rafraîchir
	newTokens, err := client.RefreshAccessToken(context.Background())
	if err != nil {
		t.Fatalf("RefreshAccessToken() error: %v", err)
	}

	// Vérifier le form envoyé
	if got := capturedForm.Get("grant_type"); got != "refresh_token" {
		t.Errorf("grant_type=%q, want refresh_token", got)
	}
	if got := capturedForm.Get("client_id"); got != "ztna-client" {
		t.Errorf("client_id=%q, want ztna-client", got)
	}
	if got := capturedForm.Get("refresh_token"); got != "old-ref-456" {
		t.Errorf("refresh_token=%q, want old-ref-456", got)
	}

	// Vérifier les nouveaux tokens
	if newTokens.AccessToken != "new-acc-789" {
		t.Errorf("AccessToken=%q, want new-acc-789", newTokens.AccessToken)
	}
	if newTokens.RefreshToken != "new-ref-101" {
		t.Errorf("RefreshToken=%q, want new-ref-101", newTokens.RefreshToken)
	}

	// Vérifier la persistance
	stored, err := client.store.Load()
	if err != nil {
		t.Fatalf("load stored tokens: %v", err)
	}
	if stored.AccessToken != "new-acc-789" {
		t.Errorf("stored AccessToken=%q, want new-acc-789", stored.AccessToken)
	}
	if stored.RefreshToken != "new-ref-101" {
		t.Errorf("stored RefreshToken=%q, want new-ref-101", stored.RefreshToken)
	}
}

// TestRefreshAccessToken_RotationKeepsOldRefresh vérifie que si le serveur
// ne renvoie pas de nouveau refresh_token, l'ancien est conservé.
func TestRefreshAccessToken_RotationKeepsOldRefresh(t *testing.T) {
	tempDir := t.TempDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Le serveur ne renvoie PAS de refresh_token dans la réponse
		_, _ = w.Write([]byte(`{
			"access_token": "rotated-acc",
			"token_type": "Bearer",
			"expires_in": 300
		}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   ts.URL + "/realms/ztna",
			ClientID: "ztna-client",
		},
		Storage: config.StorageConfig{Path: tempDir},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	_ = client.store.Save(&TokenSet{
		AccessToken:  "old-acc",
		RefreshToken: "keep-this-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-5 * time.Minute),
	})

	newTokens, err := client.RefreshAccessToken(context.Background())
	if err != nil {
		t.Fatalf("RefreshAccessToken() error: %v", err)
	}

	if newTokens.AccessToken != "rotated-acc" {
		t.Errorf("AccessToken=%q, want rotated-acc", newTokens.AccessToken)
	}
	// L'ancien refresh_token doit être conservé
	if newTokens.RefreshToken != "keep-this-refresh" {
		t.Errorf("RefreshToken=%q, want keep-this-refresh (ancien conservé)", newTokens.RefreshToken)
	}
}

// TestRefreshAccessToken_NoRefreshToken vérifie l'erreur quand il n'y a
// pas de refresh_token stocké.
func TestRefreshAccessToken_NoRefreshToken(t *testing.T) {
	tempDir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   "http://example.local/realms/ztna",
			ClientID: "ztna-client",
		},
		Storage: config.StorageConfig{Path: tempDir},
	}
	client := NewClient(cfg, log)

	_ = client.store.Save(&TokenSet{
		AccessToken: "acc-no-refresh",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(-5 * time.Minute),
		// Pas de RefreshToken
	})

	_, err := client.RefreshAccessToken(context.Background())
	if err == nil {
		t.Fatal("RefreshAccessToken() should fail without refresh_token")
	}
}

// TestGetValidAccessToken_RefreshesExpiredToken vérifie le flux complet :
// token expiré → refresh automatique → nouveau token retourné.
func TestGetValidAccessToken_RefreshesExpiredToken(t *testing.T) {
	tempDir := t.TempDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "refreshed-acc",
			"refresh_token": "refreshed-ref",
			"token_type": "Bearer",
			"expires_in": 300
		}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   ts.URL + "/realms/ztna",
			ClientID: "ztna-client",
		},
		Storage: config.StorageConfig{Path: tempDir},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	_ = client.store.Save(&TokenSet{
		AccessToken:  "expired-acc",
		RefreshToken: "valid-ref",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Minute), // expiré
	})

	token, err := client.GetValidAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetValidAccessToken() error: %v", err)
	}
	if token != "refreshed-acc" {
		t.Errorf("token=%q, want refreshed-acc", token)
	}

	// Vérifier la persistance du nouveau token
	stored, err := client.store.Load()
	if err != nil {
		t.Fatalf("load stored: %v", err)
	}
	if stored.AccessToken != "refreshed-acc" {
		t.Errorf("stored token=%q, want refreshed-acc", stored.AccessToken)
	}
}

// TestRefreshAccessToken_ServerError vérifie le traitement d'une erreur serveur.
func TestRefreshAccessToken_ServerError(t *testing.T) {
	tempDir := t.TempDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token is not active"}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   ts.URL + "/realms/ztna",
			ClientID: "ztna-client",
		},
		Storage: config.StorageConfig{Path: tempDir},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	_ = client.store.Save(&TokenSet{
		AccessToken:  "old-acc",
		RefreshToken: "revoked-ref",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-5 * time.Minute),
	})

	_, err := client.RefreshAccessToken(context.Background())
	if err == nil {
		t.Fatal("RefreshAccessToken() should fail on 401")
	}

	// Vérifier que les anciens tokens ne sont PAS écrasés
	stored, err := client.store.Load()
	if err != nil {
		t.Fatalf("load stored: %v", err)
	}
	if stored.AccessToken != "old-acc" {
		t.Errorf("stored token=%q should still be old-acc", stored.AccessToken)
	}
}

// TestWorkflowTheory_LoginThenRefreshThenGetValid simule le cycle complet :
// login → token expire → refresh → GetValidAccessToken retourne le nouveau.
func TestWorkflowTheory_LoginThenRefreshThenGetValid(t *testing.T) {
	tempDir := t.TempDir()

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")

		switch form.Get("grant_type") {
		case "password":
			_, _ = w.Write([]byte(`{
				"access_token": "login-acc",
				"refresh_token": "login-ref",
				"token_type": "Bearer",
				"expires_in": 1
			}`))
		case "refresh_token":
			_, _ = w.Write([]byte(`{
				"access_token": "refreshed-acc",
				"refresh_token": "refreshed-ref",
				"token_type": "Bearer",
				"expires_in": 300
			}`))
		default:
			t.Fatalf("unexpected grant_type: %s", form.Get("grant_type"))
		}
	}))
	defer ts.Close()

	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   ts.URL + "/realms/ztna",
			ClientID: "ztna-client",
		},
		Storage: config.StorageConfig{Path: tempDir},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	// Étape 1 : Login
	_, err := client.LoginPasswordGrantLAB(context.Background(), "alice", "Secret!23")
	if err != nil {
		t.Fatalf("Login error: %v", err)
	}

	// Simuler l'expiration du token en écrasant le fichier
	stored, _ := client.store.Load()
	stored.ExpiresAt = time.Now().Add(-1 * time.Minute) // forcer expiration
	_ = client.store.Save(stored)

	// Étape 2 : GetValidAccessToken doit déclencher le refresh
	token, err := client.GetValidAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetValidAccessToken error: %v", err)
	}
	if token != "refreshed-acc" {
		t.Errorf("token=%q, want refreshed-acc", token)
	}

	// Vérifier le fichier final
	final, _ := client.store.Load()
	if final.AccessToken != "refreshed-acc" {
		t.Errorf("final AccessToken=%q, want refreshed-acc", final.AccessToken)
	}
	if final.RefreshToken != "refreshed-ref" {
		t.Errorf("final RefreshToken=%q, want refreshed-ref", final.RefreshToken)
	}
}
