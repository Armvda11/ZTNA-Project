package oidc

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"client/internal/config"
)

// TestLoginFlow teste le workflow complet de login OIDC avec un mock server.
func TestLoginFlow(t *testing.T) {
	tempDir := t.TempDir()

	var captured url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"flow-acc","refresh_token":"flow-ref","token_type":"Bearer","expires_in":300}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   ts.URL + "/realms/ztna",
			ClientID: "test-client",
		},
		Storage: config.StorageConfig{Path: tempDir},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	tokenSet, err := client.LoginPasswordGrantLAB(context.Background(), "test-user", "test-password")
	if err != nil {
		t.Fatalf("LoginPasswordGrantLAB() error = %v", err)
	}
	if tokenSet == nil {
		t.Fatal("LoginPasswordGrantLAB() returned nil tokenSet")
	}
	if tokenSet.AccessToken == "" {
		t.Error("TokenSet has empty AccessToken")
	}
	if captured.Get("grant_type") != "password" {
		t.Errorf("grant_type=%q, want password", captured.Get("grant_type"))
	}
	if captured.Get("username") != "test-user" {
		t.Errorf("username=%q, want test-user", captured.Get("username"))
	}
}

// TestTokenRefresh teste le workflow de rafraîchissement de token avec mock.
func TestTokenRefresh(t *testing.T) {
	tempDir := t.TempDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-refreshed","refresh_token":"new-ref","token_type":"Bearer","expires_in":300}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   ts.URL + "/realms/ztna",
			ClientID: "test-client",
		},
		Storage: config.StorageConfig{Path: tempDir},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	// Stocker un token expiré avec un refresh_token
	_ = client.store.Save(&TokenSet{
		AccessToken:  "expired-access-token",
		RefreshToken: "initial-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-10 * time.Minute),
	})

	newTokenSet, err := client.RefreshAccessToken(context.Background())
	if err != nil {
		t.Fatalf("RefreshAccessToken() error = %v", err)
	}
	if newTokenSet == nil {
		t.Fatal("RefreshAccessToken() returned nil tokenSet")
	}
	if newTokenSet.AccessToken == "" {
		t.Error("RefreshAccessToken() returned empty token")
	}
	if newTokenSet.AccessToken == "expired-access-token" {
		t.Error("RefreshAccessToken() should return new token, not old token")
	}
}

// TestTokenValidation teste la détection de token absent.
func TestTokenValidation(t *testing.T) {
	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   "https://auth.example.com",
			ClientID: "test-client",
		},
		Storage: config.StorageConfig{Path: t.TempDir()},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client := NewClient(cfg, log)

	ctx := context.Background()

	token, err := client.GetValidAccessToken(ctx)
	if err == nil {
		t.Log("GetValidAccessToken() devrait échouer sans tokens stockés")
	}
	if token != "" {
		t.Error("GetValidAccessToken() should return empty token when not authenticated")
	}
}

// TestTokenStorage teste la persistance des tokens (Save/Load).
func TestTokenStorage(t *testing.T) {
	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   "https://auth.example.com",
			ClientID: "test-client",
		},
		Storage: config.StorageConfig{
			Path: t.TempDir(),
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client := NewClient(cfg, log)

	testTokenSet := &TokenSet{
		AccessToken:  "test-access-token-12345",
		RefreshToken: "test-refresh-token-67890",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}

	err := client.store.Save(testTokenSet)
	if err != nil {
		t.Fatalf("store.Save() error = %v", err)
	}

	loadedTokenSet, err := client.store.Load()
	if err != nil {
		t.Fatalf("store.Load() error = %v", err)
	}
	if loadedTokenSet.AccessToken != testTokenSet.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loadedTokenSet.AccessToken, testTokenSet.AccessToken)
	}
	if loadedTokenSet.RefreshToken != testTokenSet.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loadedTokenSet.RefreshToken, testTokenSet.RefreshToken)
	}
}
