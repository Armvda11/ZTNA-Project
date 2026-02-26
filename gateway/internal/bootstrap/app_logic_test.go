package app

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"ztna-gateway/internal/config"
	"ztna-gateway/internal/infra/session"
)

// newTestConfig crée une Config minimale pour les tests (sans fichiers TLS réels).
func newTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			ListenAddr: "127.0.0.1:19443",
			TLS: config.ServerTLSConfig{
				CertFile:     "/tmp/nonexistent-server.crt",
				KeyFile:      "/tmp/nonexistent-server.key",
				ClientCAFile: "/tmp/nonexistent-ca.crt",
			},
		},
		ControlPlane: config.ControlPlaneConfig{
			BaseURL:            "https://127.0.0.1:19080",
			InsecureSkipVerify: true,
		},
		PEP: config.PEPConfig{
			ID:    "ztna-gw-01",
			Token: "ztna-lab-pep-secret-2026",
		},
		Proxy: config.ProxyConfig{
			DialTimeout: "10s",
			MaxConns:    100,
		},
	}
}

// TestCompleteGatewayWorkflow vérifie que New() réussit mais Run() échoue sans certificats TLS.
func TestCompleteGatewayWorkflow(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// New() ne charge pas les certs TLS → doit toujours réussir
	app, err := New(ctx, cfg, log)
	if err != nil {
		t.Fatalf("New() erreur inattendue (les certs ne sont pas chargés au New) : %v", err)
	}
	if app == nil {
		t.Fatal("New() ne doit pas retourner nil")
	}

	// Run() doit échouer car les fichiers TLS n'existent pas
	runCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	err = app.Run(runCtx)
	if err == nil {
		t.Error("Run() devrait échouer sans fichiers TLS valides")
	}
	t.Logf("Run() a retourné l'erreur attendue : %v", err)
}

// TestGatewayGracefulShutdown vérifie que Close() s'exécute sans panique même sans listener démarré.
func TestGatewayGracefulShutdown(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app, err := New(ctx, cfg, log)
	if err != nil {
		t.Fatalf("New() : %v", err)
	}

	// Close() sans Run() ne doit pas paniquer
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = app.Close(shutdownCtx)
	// Une erreur est acceptable ici (listener non démarré), mais pas de panic
	t.Logf("Close() sans Run() : %v (attendu: pas de panic)", err)
}

// TestGatewayConnectionLimit vérifie que le session manager intégré impose la limite de sessions.
func TestGatewayConnectionLimit(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig()
	cfg.Proxy.MaxConns = 10
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app, err := New(ctx, cfg, log)
	if err != nil {
		t.Fatalf("New() : %v", err)
	}

	// Accéder au session manager interne (package app = accès aux champs non exportés)
	mgr := app.sessions
	if mgr == nil {
		t.Fatal("session manager interne ne doit pas être nil")
	}

	// Remplir jusqu'à la limite pour un sujet
	const maxSessions = 10
	sub := "user|limit-test"
	for i := 0; i < maxSessions; i++ {
		sess := &session.Session{
			Sub:          sub,
			Username:     "limit-test",
			ResourceType: "http",
			ResourceHost: "lan-app",
			ResourcePort: 80,
		}
		_, err := mgr.Register(sess, func() {})
		if err != nil {
			t.Fatalf("Register session %d/%d : %v", i+1, maxSessions, err)
		}
	}

	// La session suivante doit être refusée
	_, err = mgr.Register(&session.Session{Sub: sub, ResourceHost: "lan-app", ResourcePort: 80}, func() {})
	if err == nil {
		t.Errorf("Register au-delà de la limite devrait échouer (MaxConns=%d)", maxSessions)
	}

	if count := mgr.ActiveCountForSubject(sub); count != maxSessions {
		t.Errorf("ActiveCountForSubject() = %d, attendu %d", count, maxSessions)
	}
}

// TestGatewayHealthCheck vérifie que tous les sous-composants sont initialisés après New().
func TestGatewayHealthCheck(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app, err := New(ctx, cfg, log)
	if err != nil {
		t.Fatalf("New() : %v", err)
	}

	// Vérifier que les composants clés sont non-nil
	if app.authz == nil {
		t.Error("authz client doit être initialisé")
	}
	if app.proxy == nil {
		t.Error("tcp proxy doit être initialisé")
	}
	if app.sessions == nil {
		t.Error("session manager doit être initialisé")
	}
	if app.crl == nil {
		t.Error("crl store doit être initialisé")
	}
	if app.handler == nil {
		t.Error("connect handler doit être initialisé")
	}
	if app.listener == nil {
		t.Error("mtls listener doit être initialisé")
	}

	// Aucune session active au démarrage
	if count := app.sessions.ActiveCount(); count != 0 {
		t.Errorf("ActiveCount() = %d, attendu 0 au démarrage", count)
	}
}

// TestGatewayMetrics vérifie que les métriques de sessions sont correctement gérées.
func TestGatewayMetrics(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app, err := New(ctx, cfg, log)
	if err != nil {
		t.Fatalf("New() : %v", err)
	}

	mgr := app.sessions
	if mgr.ActiveCount() != 0 {
		t.Errorf("ActiveCount() = %d, attendu 0 au démarrage", mgr.ActiveCount())
	}

	// Simuler des sessions de différents utilisateurs
	users := []string{"user|alice", "user|bob", "user|charlie"}
	for _, sub := range users {
		for i := 0; i < 2; i++ {
			sess := &session.Session{Sub: sub, ResourceHost: "lan-app", ResourcePort: 80 + i}
			mgr.Register(sess, func() {}) //nolint
		}
	}

	if total := mgr.ActiveCount(); total != len(users)*2 {
		t.Errorf("ActiveCount() = %d, attendu %d", total, len(users)*2)
	}
	for _, sub := range users {
		if n := mgr.ActiveCountForSubject(sub); n != 2 {
			t.Errorf("ActiveCountForSubject(%s) = %d, attendu 2", sub, n)
		}
	}
}

// TestGatewayReloadConfig vérifie que la configuration se charge correctement via config.Load.
func TestGatewayReloadConfig(t *testing.T) {
	// Créer un fichier de config temporaire valide
	configContent := `
server:
  listen_addr: "0.0.0.0:4433"
  tls:
    cert_file: "./certs/gateway.crt"
    key_file: "./certs/gateway.key"
    client_ca_file: "./certs/client-ca.crt"
control_plane:
  base_url: "https://10.10.20.30:8080"
  insecure_skip_verify: false
pep:
  id: "ztna-gw-01"
  token: "ztna-lab-pep-secret-2026"
proxy:
  dial_timeout: "10s"
  max_conns: 100
`
	tmpFile, err := os.CreateTemp("", "ztna-gw-config-*.yaml")
	if err != nil {
		t.Fatalf("CreateTemp : %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("WriteString : %v", err)
	}
	tmpFile.Close()

	cfg, err := config.Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("config.Load() : %v", err)
	}

	if cfg.Server.ListenAddr != "0.0.0.0:4433" {
		t.Errorf("ListenAddr = %q, attendu %q", cfg.Server.ListenAddr, "0.0.0.0:4433")
	}
	if cfg.PEP.ID != "ztna-gw-01" {
		t.Errorf("PEP.ID = %q, attendu %q", cfg.PEP.ID, "ztna-gw-01")
	}
	if cfg.PEP.Token != "ztna-lab-pep-secret-2026" {
		t.Errorf("PEP.Token = %q, attendu %q", cfg.PEP.Token, "ztna-lab-pep-secret-2026")
	}
	if cfg.Proxy.MaxConns != 100 {
		t.Errorf("Proxy.MaxConns = %d, attendu 100", cfg.Proxy.MaxConns)
	}
}

