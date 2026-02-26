package credentials

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	"client/internal/config"
	"client/internal/core/domain"
)

// TestCertificateRequest teste le workflow complet de demande de certificat
// avec un mock CP qui signe réellement le CSR.
func TestCertificateRequest(t *testing.T) {
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

	accessToken := "valid-access-token"
	err := client.RequestMTLSCertFromCP(accessToken)
	if err != nil {
		t.Fatalf("RequestMTLSCertFromCP() error = %v", err)
	}

	// Vérifier que les fichiers existent
	certPath := tempDir + "/client.crt"
	keyPath := tempDir + "/client.key"

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Errorf("Certificate file not created at %s", certPath)
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Errorf("Private key file not created at %s", keyPath)
	}
}

// TestCertificateExpiration teste la détection de certificat expiré
// lors du chargement via LoadCertAndKey.
func TestCertificateExpiration(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Path: tempDir,
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	// Créer un certificat expiré
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Expired CA"},
		NotBefore:             time.Now().Add(-48 * time.Hour),
		NotAfter:              time.Now().Add(-24 * time.Hour), // expiré hier
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caCertDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caCertDER)

	clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "expired-client"},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     time.Now().Add(-24 * time.Hour), // expiré hier
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientCertDER, _ := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyDER, _ := x509.MarshalECPrivateKey(clientKey)
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})

	// Sauvegarder directement via le certStore (pas de validation X509KeyPair)
	_ = client.certStore.SaveCertAndKey(clientCertPEM, clientKeyPEM)

	// LoadCertAndKey doit détecter l'expiration
	_, _, err := client.LoadCertAndKey()
	if err == nil {
		t.Fatal("LoadCertAndKey() should fail for expired certificate")
	}
	if !errors.Is(err, domain.ErrCertExpired) {
		t.Errorf("error should be ErrCertExpired, got: %v", err)
	}
}

// TestCertificateKeyMatch teste le refus de sauvegarder un cert/key incompatible.
func TestCertificateKeyMatch(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Path: t.TempDir(),
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	// Générer deux paires distinctes — cert de l'une, key de l'autre
	ca := newTestCA(t)

	key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cert1Template := &x509.Certificate{
		SerialNumber: big.NewInt(10),
		Subject:      pkix.Name{CommonName: "client-1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	cert1DER, _ := x509.CreateCertificate(rand.Reader, cert1Template, ca.cert, &key1.PublicKey, ca.key)
	cert1PEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert1DER})

	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key2DER, _ := x509.MarshalECPrivateKey(key2)
	key2PEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key2DER})

	// cert1 + key2 → mismatch
	err := client.SaveCertAndKey(cert1PEM, key2PEM)
	if err == nil {
		t.Error("SaveCertAndKey() should return error for mismatched cert/key")
	}
}

// TestCertificatePermissions teste les permissions des fichiers (Unix uniquement).
func TestCertificatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix permissions test on Windows")
	}

	ca := newTestCA(t)
	clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(44),
		Subject:      pkix.Name{CommonName: "perm-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	clientCertDER, _ := x509.CreateCertificate(rand.Reader, clientTemplate, ca.cert, &clientKey.PublicKey, ca.key)
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyDER, _ := x509.MarshalECPrivateKey(clientKey)
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})

	tempDir := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{Path: tempDir},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(cfg, log)

	err := client.SaveCertAndKey(clientCertPEM, clientKeyPEM)
	if err != nil {
		t.Fatalf("SaveCertAndKey() error = %v", err)
	}

	keyPath := tempDir + "/client.key"
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("Private key permissions = %o, want 0600", mode)
	}
}

// Helper — pas besoin de createExpiredTestCert car on génère de vrais certs.
