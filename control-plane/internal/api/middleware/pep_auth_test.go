package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"control-plane/internal/config"
)

func pepAuthMiddleware(tokens map[string]string, mode string) *PEPAuth {
	return NewPEPAuth(config.PEPConfig{
		Tokens:   tokens,
		AuthMode: mode,
	})
}

func nextOK(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func doRequest(t *testing.T, handler http.HandlerFunc, pepID, token string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/pep/authorize", nil)
	if pepID != "" {
		req.Header.Set("X-PEP-ID", pepID)
	}
	if token != "" {
		req.Header.Set("X-PEP-TOKEN", token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}

func TestPEPAuth_ValidToken_Passes(t *testing.T) {
	auth := pepAuthMiddleware(map[string]string{"gw-1": "secret-token"}, "token")
	h := auth.RequirePEP(http.HandlerFunc(nextOK))
	code := doRequest(t, h.ServeHTTP, "gw-1", "secret-token")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
}

func TestPEPAuth_WrongToken_Rejects(t *testing.T) {
	auth := pepAuthMiddleware(map[string]string{"gw-1": "secret-token"}, "token")
	h := auth.RequirePEP(http.HandlerFunc(nextOK))
	code := doRequest(t, h.ServeHTTP, "gw-1", "wrong-token")
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

func TestPEPAuth_EmptyToken_Rejects(t *testing.T) {
	auth := pepAuthMiddleware(map[string]string{"gw-1": "secret-token"}, "token")
	h := auth.RequirePEP(http.HandlerFunc(nextOK))
	code := doRequest(t, h.ServeHTTP, "gw-1", "")
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

func TestPEPAuth_MissingPEPID_Rejects(t *testing.T) {
	auth := pepAuthMiddleware(map[string]string{"gw-1": "secret-token"}, "token")
	h := auth.RequirePEP(http.HandlerFunc(nextOK))
	code := doRequest(t, h.ServeHTTP, "", "secret-token")
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

func TestPEPAuth_UnknownPEPID_Rejects(t *testing.T) {
	auth := pepAuthMiddleware(map[string]string{"gw-1": "secret-token"}, "token")
	h := auth.RequirePEP(http.HandlerFunc(nextOK))
	code := doRequest(t, h.ServeHTTP, "gw-unknown", "secret-token")
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

// Ensure that a token that is a prefix of the real token is rejected.
// This is the canonical timing-attack scenario.
func TestPEPAuth_PrefixToken_Rejects(t *testing.T) {
	auth := pepAuthMiddleware(map[string]string{"gw-1": "secret-token"}, "token")
	h := auth.RequirePEP(http.HandlerFunc(nextOK))
	code := doRequest(t, h.ServeHTTP, "gw-1", "secret")
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

func TestPEPAuth_EmptyTokenMap_Rejects(t *testing.T) {
	auth := pepAuthMiddleware(nil, "token")
	h := auth.RequirePEP(http.HandlerFunc(nextOK))
	code := doRequest(t, h.ServeHTTP, "gw-1", "secret-token")
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

func TestPEPAuth_RevokedPEP_Forbidden(t *testing.T) {
	auth := NewPEPAuth(config.PEPConfig{
		AuthMode:      "token",
		Tokens:        map[string]string{"gw-1": "secret-token"},
		RevokedPEPIDs: []string{"gw-1"},
	})
	h := auth.RequirePEP(http.HandlerFunc(nextOK))
	code := doRequest(t, h.ServeHTTP, "gw-1", "secret-token")
	if code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", code)
	}
}
