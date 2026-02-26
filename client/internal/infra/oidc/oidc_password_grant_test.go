package oidc

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"client/internal/config"
)

func TestLoginPasswordGrantLAB_Success(t *testing.T) {
	tempDir := t.TempDir()

	var captured url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realms/ztna/protocol/openid-connect/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		captured, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"acc-123","refresh_token":"ref-456","token_type":"Bearer","expires_in":300}`))
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

	tokens, err := client.LoginPasswordGrantLAB(context.Background(), "alice", "Password123!")
	if err != nil {
		t.Fatalf("LoginPasswordGrantLAB error: %v", err)
	}
	if tokens == nil || tokens.AccessToken == "" {
		t.Fatalf("expected non-empty access token")
	}
	if got := captured.Get("grant_type"); got != "password" {
		t.Fatalf("grant_type=%q, want password", got)
	}
	if got := captured.Get("client_id"); got != "ztna-client" {
		t.Fatalf("client_id=%q, want ztna-client", got)
	}
	if got := captured.Get("username"); got != "alice" {
		t.Fatalf("username=%q, want alice", got)
	}

	storedPath := filepath.Join(tempDir, "tokens.json")
	stored, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("read stored tokens: %v", err)
	}
	var decoded TokenSet
	if err := json.Unmarshal(stored, &decoded); err != nil {
		t.Fatalf("decode stored tokens: %v", err)
	}
	if decoded.AccessToken != "acc-123" {
		t.Fatalf("stored access token=%q, want acc-123", decoded.AccessToken)
	}
}

func TestGetValidAccessToken_FromStoredToken(t *testing.T) {
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

	err := client.store.Save(&TokenSet{
		AccessToken:  "acc-valid",
		RefreshToken: "ref-valid",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("save tokens: %v", err)
	}

	token, err := client.GetValidAccessToken(context.Background())
	if err != nil {
		t.Fatalf("GetValidAccessToken error: %v", err)
	}
	if token != "acc-valid" {
		t.Fatalf("token=%q, want acc-valid", token)
	}
}

func TestGetValidAccessToken_ExpiredWithoutRefreshFails(t *testing.T) {
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

	err := client.store.Save(&TokenSet{
		AccessToken: "acc-expired",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("save tokens: %v", err)
	}

	_, err = client.GetValidAccessToken(context.Background())
	if err == nil {
		t.Fatal("expected error for expired token without refresh token")
	}
}
