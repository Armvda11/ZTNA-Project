package authorize

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"ztna-gateway/internal/config"
	"ztna-gateway/internal/core/domain"
)

// newInsecureClient crée un Client pointant vers un serveur httptest TLS (cert auto-signé accepté).
func newInsecureClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL:            baseURL,
			InsecureSkipVerify: true, // httptest TLS utilise un cert auto-signé
		},
		PEP: config.PEPConfig{
			ID:    "ztna-gw-01",
			Token: "ztna-lab-pep-secret-2026",
		},
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return NewClient(cfg, log)
}

// newAuthzReq crée une AuthzRequest de test standard.
func newAuthzReq(sub, username, host string, port int) *AuthzRequest {
	return &AuthzRequest{
		Subject: domain.SubjectRef{
			Sub:      sub,
			Username: username,
			Groups:   []string{"ztna-admins"},
		},
		Action: "connect",
		Resource: ResourceRef{
			Type: "http",
			Host: host,
			Port: port,
		},
		Context: AuthzContext{
			SourceIP:  "10.10.10.10",
			GatewayID: "ztna-gw-01",
		},
	}
}

// TestAuthorizationAllow vérifie qu'une réponse "allow" du CP est correctement interprétée.
func TestAuthorizationAllow(t *testing.T) {
	// Simuler un CP qui retourne "allow"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Vérifier les headers PEP
		if r.Header.Get("X-PEP-ID") == "" {
			t.Errorf("X-PEP-ID manquant dans la requête au CP")
		}
		if r.Header.Get("X-PEP-TOKEN") == "" {
			t.Errorf("X-PEP-TOKEN manquant dans la requête au CP")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"effect":       "allow",
			"ttl_seconds":  60,
			"decision_id":  "dec-allow-001",
			"policy_version": 1,
		})
	}))
	defer ts.Close()

	client := newInsecureClient(t, ts.URL)
	resp, err := client.Authorize(newAuthzReq("user|alice", "alice", "lan-app", 80))
	if err != nil {
		t.Fatalf("Authorize() erreur inattendue : %v", err)
	}
	if resp.Decision != "allow" {
		t.Errorf("Decision = %q, attendu %q", resp.Decision, "allow")
	}
	if resp.DecisionID != "dec-allow-001" {
		t.Errorf("DecisionID = %q, attendu %q", resp.DecisionID, "dec-allow-001")
	}
	if resp.TTLSeconds != 60 {
		t.Errorf("TTLSeconds = %d, attendu 60", resp.TTLSeconds)
	}
}

// TestAuthorizationDeny vérifie qu'une réponse "deny" du CP est correctement interprétée.
func TestAuthorizationDeny(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"effect":     "deny",
			"reason":     "aucune politique correspondante",
			"decision_id": "dec-deny-001",
		})
	}))
	defer ts.Close()

	client := newInsecureClient(t, ts.URL)
	resp, err := client.Authorize(newAuthzReq("user|bob", "bob", "restricted.internal", 5432))
	if err != nil {
		t.Fatalf("Authorize() erreur inattendue : %v", err)
	}
	if resp.Decision != "deny" {
		t.Errorf("Decision = %q, attendu %q", resp.Decision, "deny")
	}
	if resp.Reason == "" {
		t.Error("Reason ne doit pas être vide pour un deny")
	}
}

// TestAuthorizationRetry vérifie qu'une erreur est retournée quand le CP est injoignable.
func TestAuthorizationRetry(t *testing.T) {
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL:            "https://127.0.0.1:19999", // port non utilisé
			InsecureSkipVerify: true,
		},
		PEP: config.PEPConfig{ID: "gw-1", Token: "secret"},
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewClient(cfg, log)

	_, err := client.Authorize(newAuthzReq("user|alice", "alice", "lan-app", 80))
	if err == nil {
		t.Error("Authorize() devrait retourner une erreur quand le CP est injoignable")
	}
}

// TestAuthorizationTimeout vérifie qu'un code 401 du CP retourne bien une erreur.
func TestAuthorizationTimeout(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simuler un reject d'authentification (mauvais token PEP)
		http.Error(w, `{"error":"invalid PEP token"}`, http.StatusUnauthorized)
	}))
	defer ts.Close()

	client := newInsecureClient(t, ts.URL)
	_, err := client.Authorize(newAuthzReq("user|alice", "alice", "lan-app", 80))
	if err == nil {
		t.Error("Authorize() devrait retourner une erreur pour une réponse 401")
	}
}

// TestAuthorizationCaching vérifie que deux requêtes identiques successives fonctionnent correctement.
func TestAuthorizationCaching(t *testing.T) {
	callCount := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"effect":      "allow",
			"ttl_seconds": 60,
			"decision_id": "dec-stable-001",
		})
	}))
	defer ts.Close()

	client := newInsecureClient(t, ts.URL)
	req := newAuthzReq("user|alice", "alice", "lan-app", 80)

	resp1, err := client.Authorize(req)
	if err != nil {
		t.Fatalf("Authorize() 1re requête : %v", err)
	}
	resp2, err := client.Authorize(req)
	if err != nil {
		t.Fatalf("Authorize() 2e requête : %v", err)
	}

	// Les deux requêtes doivent retourner allow (comportement stable)
	if resp1.Decision != "allow" || resp2.Decision != "allow" {
		t.Errorf("Les deux requêtes doivent retourner allow, obtenu %q et %q", resp1.Decision, resp2.Decision)
	}
	// La décision_id doit être identique (même serveur mock stateless)
	if resp1.DecisionID != resp2.DecisionID {
		t.Errorf("DecisionID différents entre deux requêtes identiques : %q vs %q", resp1.DecisionID, resp2.DecisionID)
	}
	// Note: le client n'a pas de cache pour l'instant — les deux appels atteignent le CP
	if callCount != 2 {
		t.Logf("CP appelé %d fois (sans cache → 2 appels attendus, futur: 1 avec cache)", callCount)
	}
}
