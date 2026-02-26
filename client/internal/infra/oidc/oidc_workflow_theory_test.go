package oidc

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"client/internal/config"
)

func TestWorkflowTheory_LoginThenGetValidToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realms/ztna/protocol/openid-connect/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"wan-token","refresh_token":"wan-refresh","token_type":"Bearer","expires_in":600}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   ts.URL + "/realms/ztna",
			ClientID: "ztna-client",
		},
		Storage: config.StorageConfig{Path: t.TempDir()},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	_, err := client.LoginPasswordGrantLAB(context.Background(), "alice", "Password123!")
	if err != nil {
		t.Fatalf("login password grant failed: %v", err)
	}

	token, err := client.GetValidAccessToken(context.Background())
	if err != nil {
		t.Fatalf("expected valid token after login, got error: %v", err)
	}
	if token != "wan-token" {
		t.Fatalf("token=%q, want wan-token", token)
	}
}
