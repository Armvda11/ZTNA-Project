package credentials

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"client/internal/config"
)

func TestNewClient(t *testing.T) {
	cfg := &config.Config{}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	client := NewClient(cfg, log)

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.cfg != cfg {
		t.Error("NewClient() did not store config")
	}
	if client.http == nil {
		t.Error("NewClient() did not initialize HTTP client")
	}
	if client.certStore == nil {
		t.Error("NewClient() did not initialize cert store")
	}
}

// testCA regroupe les artefacts d'une CA de test pour signer des certificats.
type testCA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte
}

// newTestCA crée une CA auto-signée pour les tests.
func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	cert, _ := x509.ParseCertificate(certDER)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return &testCA{cert: cert, key: key, certPEM: certPEM}
}

// mockCPHandler retourne un handler HTTP qui simule l'endpoint
// POST /api/v1/credentials/mtls-cert du Control Plane.
// Il parse le CSR, signe un certificat avec la CA de test, et le retourne.
func mockCPHandler(t *testing.T, ca *testCA) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Vérifier le method et le path
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/v1/credentials/mtls-cert" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Vérifier le header Authorization
		auth := r.Header.Get("Authorization")
		if auth == "" || len(auth) < 8 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Parser le corps JSON
		var req struct {
			CSR string `json:"csr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Parser le CSR PEM
		block, _ := pem.Decode([]byte(req.CSR))
		if block == nil {
			http.Error(w, "invalid CSR PEM", http.StatusBadRequest)
			return
		}
		csr, err := x509.ParseCertificateRequest(block.Bytes)
		if err != nil {
			http.Error(w, "invalid CSR: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Signer le certificat
		certTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(100),
			Subject:      csr.Subject,
			NotBefore:    time.Now().Add(-time.Minute),
			NotAfter:     time.Now().Add(15 * time.Minute),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		certDER, err := x509.CreateCertificate(rand.Reader, certTemplate, ca.cert, csr.PublicKey, ca.key)
		if err != nil {
			http.Error(w, "signing error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

		resp := struct {
			Certificate string `json:"certificate"`
			ExpiresAt   string `json:"expires_at"`
		}{
			Certificate: string(certPEM),
			ExpiresAt:   certTemplate.NotAfter.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// TestRequestMTLSCertFromCP_Success teste le flux complet de demande de certificat
// avec un mock CP qui signe le CSR et retourne le certificat.
func TestRequestMTLSCertFromCP_Success(t *testing.T) {
	ca := newTestCA(t)
	ts := httptest.NewServer(mockCPHandler(t, ca))
	defer ts.Close()

	tempDir := t.TempDir()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL: ts.URL,
		},
		Storage: config.StorageConfig{
			Path: tempDir,
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	err := client.RequestMTLSCertFromCP("valid-access-token-12345")
	if err != nil {
		t.Fatalf("RequestMTLSCertFromCP() error: %v", err)
	}

	// Vérifier que le certificat et la clé sont sauvegardés
	certPath := tempDir + "/client.crt"
	keyPath := tempDir + "/client.key"

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Errorf("Certificate file not created at %s", certPath)
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Errorf("Private key file not created at %s", keyPath)
	}

	// Vérifier que le cert et la clé forment une paire valide
	certPEM, _ := os.ReadFile(certPath)
	keyPEM, _ := os.ReadFile(keyPath)
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Errorf("cert/key pair invalid: %v", err)
	}

	// Vérifier le certificat
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if cert.Subject.CommonName != "ztna-client" {
		t.Errorf("CN=%q, want ztna-client", cert.Subject.CommonName)
	}
}

// TestRequestMTLSCertFromCP_Unauthorized teste le refus par le CP (401).
func TestRequestMTLSCertFromCP_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{BaseURL: ts.URL},
		Storage:      config.StorageConfig{Path: t.TempDir()},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	err := client.RequestMTLSCertFromCP("invalid-token")
	if err == nil {
		t.Fatal("RequestMTLSCertFromCP() should fail on 401")
	}
}

// TestSaveCertAndKey_ValidPair teste la sauvegarde avec un cert/key valide.
func TestSaveCertAndKey_ValidPair(t *testing.T) {
	ca := newTestCA(t)

	// Générer un cert client signé par la CA
	clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientCertDER, _ := x509.CreateCertificate(rand.Reader, clientTemplate, ca.cert, &clientKey.PublicKey, ca.key)
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyDER, _ := x509.MarshalECPrivateKey(clientKey)
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})

	cfg := &config.Config{
		Storage: config.StorageConfig{Path: t.TempDir()},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	err := client.SaveCertAndKey(clientCertPEM, clientKeyPEM)
	if err != nil {
		t.Fatalf("SaveCertAndKey() error: %v", err)
	}
}

// TestSaveCertAndKey_MismatchedPair teste le refus d'un cert/key incompatible.
func TestSaveCertAndKey_MismatchedPair(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{Path: t.TempDir()},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	err := client.SaveCertAndKey([]byte("not-a-cert"), []byte("not-a-key"))
	if err == nil {
		t.Fatal("SaveCertAndKey() should fail with mismatched cert/key")
	}
}

// TestLoadCertAndKey_Success teste le chargement après sauvegarde.
func TestLoadCertAndKey_Success(t *testing.T) {
	ca := newTestCA(t)
	clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(43),
		Subject:      pkix.Name{CommonName: "test-load"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	clientCertDER, _ := x509.CreateCertificate(rand.Reader, clientTemplate, ca.cert, &clientKey.PublicKey, ca.key)
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyDER, _ := x509.MarshalECPrivateKey(clientKey)
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})

	cfg := &config.Config{
		Storage: config.StorageConfig{Path: t.TempDir()},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	_ = client.SaveCertAndKey(clientCertPEM, clientKeyPEM)

	loadedCert, loadedKey, err := client.LoadCertAndKey()
	if err != nil {
		t.Fatalf("LoadCertAndKey() error: %v", err)
	}
	if len(loadedCert) == 0 || len(loadedKey) == 0 {
		t.Fatal("LoadCertAndKey() returned empty data")
	}

	// Vérifier que le cert chargé est bien parseable
	if _, err := tls.X509KeyPair(loadedCert, loadedKey); err != nil {
		t.Errorf("loaded cert/key pair invalid: %v", err)
	}
}

// TestLoadCertAndKey_NotFound teste l'erreur quand aucun certificat n'existe.
func TestLoadCertAndKey_NotFound(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{Path: t.TempDir()},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	_, _, err := client.LoadCertAndKey()
	if err == nil {
		t.Fatal("LoadCertAndKey() should fail when no cert exists")
	}
}
