package protocol

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	authorize "ztna-gateway/internal/infra/authz"
	crl "ztna-gateway/internal/infra/revocation"
	"ztna-gateway/internal/infra/proxy"
	"ztna-gateway/internal/infra/session"
	"ztna-gateway/internal/config"
)

// newTestCert génère un certificat X.509 auto-signé ECDSA P-256 minimal pour les tests.
func newTestCert(t *testing.T, serial int64) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey : %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "test-user", Organization: []string{"ZTNA-Lab"}},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		// Extensions pour le CN de sujet (attendu par ExtractSubjectFromCert)
		ExtraExtensions: nil,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate : %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("ParseCertificate : %v", err)
	}
	return cert
}

// newHandlerWithCPMock crée un Handler complet avec un CP mock (httptest TLS).
func newHandlerWithCPMock(t *testing.T, cpHandler http.Handler) (*Handler, *httptest.Server) {
	t.Helper()
	ts := httptest.NewTLSServer(cpHandler)

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL:            ts.URL,
			InsecureSkipVerify: true,
		},
		PEP: config.PEPConfig{ID: "ztna-gw-01", Token: "ztna-lab-pep-secret-2026"},
		Proxy: config.ProxyConfig{DialTimeout: "5s", MaxConns: 100},
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	authzClient   := authorize.NewClient(cfg, log)
	tcpProxy      := proxy.NewTCPProxy(cfg, log)
	sessionMgr    := session.NewManager(log)
	crlStore      := crl.NewStore()

	h := NewHandler(authzClient, tcpProxy, sessionMgr, crlStore, log, "ztna-gw-test")
	return h, ts
}

// sendConnectRequest envoie une ConnectRequest encadrée (4 bytes length + JSON) sur conn.
func sendConnectRequest(t *testing.T, conn net.Conn, req ConnectRequest) {
	t.Helper()
	conn.SetDeadline(time.Now().Add(5 * time.Second)) //nolint
	if err := WriteMessage(conn, req); err != nil {
		t.Fatalf("WriteMessage (client→GW) : %v", err)
	}
}

// readConnectResponse lit et décode une ConnectResponse encadrée depuis conn.
func readConnectResponse(t *testing.T, conn net.Conn) ConnectResponse {
	t.Helper()
	conn.SetDeadline(time.Now().Add(5 * time.Second)) //nolint
	var resp ConnectResponse
	if err := ReadMessage(conn, &resp); err != nil {
		t.Fatalf("ReadMessage (GW→client) : %v", err)
	}
	return resp
}

// TestHandleConnectRequest vérifie qu'une requête avec une ressource incomplète est refusée.
func TestHandleConnectRequest(t *testing.T) {
	h, ts := newHandlerWithCPMock(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ne doit jamais être appelé (validation avant authz)
		t.Error("le CP ne devrait pas être appelé pour une ressource incomplète")
	}))
	defer ts.Close()

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	cert := newTestCert(t, 0x1111)

	go h.HandleConnection(serverSide, cert)

	// Envoyer une requête sans port → ressource incomplète
	sendConnectRequest(t, clientSide, ConnectRequest{
		ProtocolVersion: 1,
		Action:          "connect",
		Resource:        ResourceTarget{Type: "http", Host: "lan-app", Port: 0}, // port manquant
	})

	resp := readConnectResponse(t, clientSide)
	if resp.Decision != "deny" {
		t.Errorf("Decision = %q, attendu %q", resp.Decision, "deny")
	}
	if resp.Reason == "" {
		t.Error("Reason ne doit pas être vide pour un deny de ressource incomplète")
	}
}

// TestHandleConnectAllow vérifie le flux complet quand le CP retourne "allow".
func TestHandleConnectAllow(t *testing.T) {
	// Démarrer un serveur cible (simule lan-app)
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen target : %v", err)
	}
	defer target.Close()
	targetAddr, _ := net.ResolveTCPAddr("tcp", target.Addr().String())

	// Accepter et fermer immédiatement (le proxy va se terminer en EOF)
	go func() {
		for {
			c, err := target.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	cpCalled := false
	h, ts := newHandlerWithCPMock(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cpCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"effect":      "allow",
			"ttl_seconds": 10,
			"decision_id": "dec-test-allow",
		})
	}))
	defer ts.Close()

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	cert := newTestCert(t, 0x2222)

	go h.HandleConnection(serverSide, cert)

	sendConnectRequest(t, clientSide, ConnectRequest{
		ProtocolVersion: 1,
		Action:          "connect",
		Resource:        ResourceTarget{Type: "http", Host: "127.0.0.1", Port: targetAddr.Port},
	})

	resp := readConnectResponse(t, clientSide)
	if resp.Decision != "allow" {
		t.Errorf("Decision = %q, attendu %q (CP a retourné allow)", resp.Decision, "allow")
	}
	if resp.DecisionID != "dec-test-allow" {
		t.Errorf("DecisionID = %q, attendu %q", resp.DecisionID, "dec-test-allow")
	}
	if !cpCalled {
		t.Error("le CP d'autorisation n'a pas été appelé")
	}
}

// TestHandleConnectDeny vérifie que le flux CONNECT retourne "deny" quand le CP refuse.
func TestHandleConnectDeny(t *testing.T) {
	h, ts := newHandlerWithCPMock(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"effect":     "deny",
			"reason":     "aucune politique correspondante pour cette ressource",
			"decision_id": "dec-test-deny",
		})
	}))
	defer ts.Close()

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	cert := newTestCert(t, 0x3333)

	go h.HandleConnection(serverSide, cert)

	sendConnectRequest(t, clientSide, ConnectRequest{
		ProtocolVersion: 1,
		Action:          "connect",
		Resource:        ResourceTarget{Type: "http", Host: "forbidden.internal", Port: 443},
	})

	resp := readConnectResponse(t, clientSide)
	if resp.Decision != "deny" {
		t.Errorf("Decision = %q, attendu %q", resp.Decision, "deny")
	}
	if resp.Reason == "" {
		t.Error("Reason doit être fournie pour un deny")
	}
}

// TestHandleMalformedRequest vérifie que des octets invalides ferment proprement la connexion.
func TestHandleMalformedRequest(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	h := NewHandler(nil, nil, session.NewManager(log), crl.NewStore(), log, "ztna-gw-test")

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	cert := newTestCert(t, 0x4444)

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.HandleConnection(serverSide, cert)
	}()

	// Envoyer 4 bytes annonçant un message de 2MB (dépasse MaxMessageSize)
	// → ReadMessage doit retourner une erreur et fermer la connexion
	oversizeLen := [4]byte{0x00, 0x20, 0x00, 0x00} // 2 097 152 bytes > 1MB
	clientSide.SetDeadline(time.Now().Add(3 * time.Second)) //nolint
	clientSide.Write(oversizeLen[:])                         //nolint

	select {
	case <-done:
		// Handler a fermé la connexion à cause du message trop grand → correct
	case <-time.After(3 * time.Second):
		t.Error("HandleConnection n'a pas terminé après lecture d'un message surdimensionné")
	}
}

// TestProtocolVersion vérifie le framing length-prefixed WriteMessage/ReadMessage directement.
func TestProtocolVersion(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	// Écrire une ConnectResponse côté Gateway
	resp := ConnectResponse{
		Decision:   "allow",
		DecisionID: "dec-framing-test",
		TTLSeconds: 60,
	}

	done := make(chan error, 1)
	go func() {
		serverSide.SetDeadline(time.Now().Add(3 * time.Second)) //nolint
		done <- WriteMessage(serverSide, resp)
	}()

	// Lire la réponse côté client
	var received ConnectResponse
	clientSide.SetDeadline(time.Now().Add(3 * time.Second)) //nolint
	if err := ReadMessage(clientSide, &received); err != nil {
		t.Fatalf("ReadMessage : %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("WriteMessage : %v", err)
	}

	if received.Decision != "allow" {
		t.Errorf("Decision = %q, attendu %q", received.Decision, "allow")
	}
	if received.DecisionID != "dec-framing-test" {
		t.Errorf("DecisionID = %q, attendu %q", received.DecisionID, "dec-framing-test")
	}
	if received.TTLSeconds != 60 {
		t.Errorf("TTLSeconds = %d, attendu 60", received.TTLSeconds)
	}
}
