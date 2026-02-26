package tunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"client/internal/config"
	"client/internal/core/domain"
)

// testPKI regroupe les artefacts TLS générés pour les tests tunnel.
type testPKI struct {
	CACertPEM     []byte
	ServerCert    tls.Certificate
	ClientCert    tls.Certificate
	ClientCertPEM []byte
	ClientKeyPEM  []byte
}

// generateTestPKI crée une CA auto-signée, un certificat serveur et un certificat client.
func generateTestPKI(t *testing.T) *testPKI {
	t.Helper()

	// Clé et certificat CA
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caCert, _ := x509.ParseCertificate(caCertDER)
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	// Certificat serveur (pour la Gateway mock)
	serverKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, _ := x509.MarshalECPrivateKey(serverKey)
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})
	serverTLSCert, _ := tls.X509KeyPair(serverCertPEM, serverKeyPEM)

	// Certificat client (pour le client ZTNA)
	clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "ztna-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client cert: %v", err)
	}
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyDER, _ := x509.MarshalECPrivateKey(clientKey)
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})
	clientTLSCert, _ := tls.X509KeyPair(clientCertPEM, clientKeyPEM)

	return &testPKI{
		CACertPEM:     caCertPEM,
		ServerCert:    serverTLSCert,
		ClientCert:    clientTLSCert,
		ClientCertPEM: clientCertPEM,
		ClientKeyPEM:  clientKeyPEM,
	}
}

// startMockGateway démarre un serveur TLS qui simule la Gateway ZTNA.
// Le handler reçoit la connexion TLS post-handshake.
func startMockGateway(t *testing.T, pki *testPKI, handler func(conn net.Conn)) (address string, cleanup func()) {
	t.Helper()

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(pki.CACertPEM)

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{pki.ServerCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener fermé
			}
			go handler(conn)
		}
	}()

	return listener.Addr().String(), func() { listener.Close() }
}

// TestTunnelConnection teste l'établissement complet d'un tunnel :
// TLS dial → CONNECT handshake → décision allow.
func TestTunnelConnection(t *testing.T) {
	pki := generateTestPKI(t)

	address, cleanup := startMockGateway(t, pki, func(conn net.Conn) {
		defer conn.Close()
		// Lire la ConnectRequest
		var req ConnectRequest
		if err := ReadMessage(conn, &req); err != nil {
			t.Errorf("gateway ReadMessage: %v", err)
			return
		}
		// Répondre allow
		resp := ConnectResponse{
			Decision:   "allow",
			DecisionID: "test-dec-001",
			TTLSeconds: 3600,
		}
		if err := WriteMessage(conn, resp); err != nil {
			t.Errorf("gateway WriteMessage: %v", err)
		}
	})
	defer cleanup()

	// Écrire la CA dans un fichier temporaire pour buildTLSConfig
	caFile := t.TempDir() + "/ca.crt"
	if err := writeFile(caFile, pki.CACertPEM); err != nil {
		t.Fatalf("write CA file: %v", err)
	}

	cfg := &config.Config{
		Gateway: config.GatewayConfig{Address: address},
		TLS:     config.TLSConfig{CAFile: caFile},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := NewManager(cfg, log)

	conn, err := mgr.Connect(pki.ClientCertPEM, pki.ClientKeyPEM, "ssh://10.0.30.10:22")
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer conn.Close()

	// La connexion doit être utilisable
	deadline := time.Now().Add(5 * time.Second)
	if err := conn.SetDeadline(deadline); err != nil {
		t.Errorf("SetDeadline() error: %v", err)
	}
}

// TestTunnelDenied teste le traitement d'un refus (decision=deny).
func TestTunnelDenied(t *testing.T) {
	pki := generateTestPKI(t)

	address, cleanup := startMockGateway(t, pki, func(conn net.Conn) {
		defer conn.Close()
		var req ConnectRequest
		_ = ReadMessage(conn, &req)
		resp := ConnectResponse{
			Decision:   "deny",
			Reason:     "politique d'accès refusée",
			DecisionID: "test-dec-deny",
		}
		_ = WriteMessage(conn, resp)
	})
	defer cleanup()

	caFile := t.TempDir() + "/ca.crt"
	_ = writeFile(caFile, pki.CACertPEM)

	cfg := &config.Config{
		Gateway: config.GatewayConfig{Address: address},
		TLS:     config.TLSConfig{CAFile: caFile},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := NewManager(cfg, log)

	conn, err := mgr.Connect(pki.ClientCertPEM, pki.ClientKeyPEM, "ssh://forbidden:22")
	if err == nil {
		t.Fatal("Connect() should return error when denied")
	}
	if conn != nil {
		t.Error("Connect() should return nil connection when denied")
		conn.Close()
	}
	if !errors.Is(err, domain.ErrConnectionDenied) {
		t.Errorf("error should wrap ErrConnectionDenied, got: %v", err)
	}
}

// TestTunnelHandshake vérifie que la ConnectRequest envoyée contient
// les bons champs (resource, protocol version, action).
func TestTunnelHandshake(t *testing.T) {
	pki := generateTestPKI(t)

	var receivedReq ConnectRequest
	address, cleanup := startMockGateway(t, pki, func(conn net.Conn) {
		defer conn.Close()
		_ = ReadMessage(conn, &receivedReq)
		_ = WriteMessage(conn, ConnectResponse{Decision: "allow", DecisionID: "hsk-001"})
	})
	defer cleanup()

	caFile := t.TempDir() + "/ca.crt"
	_ = writeFile(caFile, pki.CACertPEM)

	cfg := &config.Config{
		Gateway: config.GatewayConfig{Address: address},
		TLS:     config.TLSConfig{CAFile: caFile},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := NewManager(cfg, log)

	conn, err := mgr.Connect(pki.ClientCertPEM, pki.ClientKeyPEM, "tcp://10.0.1.50:8080")
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer conn.Close()

	if receivedReq.Action != "connect" {
		t.Errorf("Action=%q, want connect", receivedReq.Action)
	}
	if receivedReq.ProtocolVersion != CurrentProtocolVersion {
		t.Errorf("ProtocolVersion=%d, want %d", receivedReq.ProtocolVersion, CurrentProtocolVersion)
	}
	if receivedReq.Resource.Type != "tcp" {
		t.Errorf("Resource.Type=%q, want tcp", receivedReq.Resource.Type)
	}
	if receivedReq.Resource.Host != "10.0.1.50" {
		t.Errorf("Resource.Host=%q, want 10.0.1.50", receivedReq.Resource.Host)
	}
	if receivedReq.Resource.Port != 8080 {
		t.Errorf("Resource.Port=%d, want 8080", receivedReq.Resource.Port)
	}
}

// TestRelayTraffic teste le relais bidirectionnel entre deux connexions.
func TestRelayTraffic(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{Address: "unused:9443"},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := NewManager(cfg, log)

	// Créer deux paires de pipes
	tunnelClient, tunnelServer := net.Pipe()
	localClient, localServer := net.Pipe()

	// Lancer le relais entre tunnelClient et localClient
	relayDone := make(chan error, 1)
	go func() {
		relayDone <- mgr.RelayTraffic(tunnelClient, localClient)
	}()

	// Écrire côté tunnel (serveur) → doit arriver côté local (serveur)
	testData := []byte("hello from tunnel")
	go func() {
		_, _ = tunnelServer.Write(testData)
		tunnelServer.Close()
	}()

	buf := make([]byte, 256)
	n, err := localServer.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read from local: %v", err)
	}
	if string(buf[:n]) != "hello from tunnel" {
		t.Errorf("received=%q, want %q", string(buf[:n]), "hello from tunnel")
	}

	localServer.Close()

	// Attendre la fin du relais
	<-relayDone
}

// TestTunnelTimeout teste l'échec de connexion vers une adresse inaccessible.
func TestTunnelTimeout(t *testing.T) {
	pki := generateTestPKI(t)
	caFile := t.TempDir() + "/ca.crt"
	_ = writeFile(caFile, pki.CACertPEM)

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Address: "127.0.0.1:1", // Port 1 — connection refused immédiat
		},
		TLS: config.TLSConfig{CAFile: caFile},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := NewManager(cfg, log)

	conn, err := mgr.Connect(pki.ClientCertPEM, pki.ClientKeyPEM, "ssh://backend:22")
	if err == nil {
		t.Error("Connect() should return error for unreachable address")
	}
	if conn != nil {
		t.Error("Connect() should return nil on failure")
		conn.Close()
	}
}

// writeFile est un helper pour écrire un fichier dans les tests.
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
