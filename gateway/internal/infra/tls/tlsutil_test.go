package tlsutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ztna-gateway/internal/config"
)

func writeTempCACertPEM(t *testing.T, dir string) string {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey error: %v", err)
	}

	tpl := &x509.Certificate{
		SerialNumber:          newSerial(t),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate error: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	path := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	return path
}

func newSerial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatalf("rand.Int serial error: %v", err)
	}
	return serial
}

func TestLoadCertPoolFromPEMFile_Valid(t *testing.T) {
	tmp := t.TempDir()
	path := writeTempCACertPEM(t, tmp)

	pool, err := LoadCertPoolFromPEMFile(path)
	if err != nil {
		t.Fatalf("LoadCertPoolFromPEMFile error: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil cert pool")
	}
}

func TestLoadCertPoolFromPEMFile_Invalid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.pem")
	if err := os.WriteFile(path, []byte("not-a-cert"), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	_, err := LoadCertPoolFromPEMFile(path)
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestNewControlPlaneHTTPClient_TokenMode(t *testing.T) {
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL:            "https://cp.example",
			AuthMode:           "token",
			InsecureSkipVerify: true,
		},
	}

	cli, err := NewControlPlaneHTTPClient(cfg, 3*time.Second)
	if err != nil {
		t.Fatalf("NewControlPlaneHTTPClient error: %v", err)
	}
	if cli == nil {
		t.Fatal("expected non-nil client")
	}
	if cli.Timeout != 3*time.Second {
		t.Fatalf("unexpected timeout: %v", cli.Timeout)
	}
}

func TestNewControlPlaneHTTPClient_MTLSMissingFiles(t *testing.T) {
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL:        "https://cp.example",
			AuthMode:       "mtls",
			ClientCertFile: "missing-cert.pem",
			ClientKeyFile:  "missing-key.pem",
		},
	}

	_, err := NewControlPlaneHTTPClient(cfg, 0)
	if err == nil {
		t.Fatal("expected error with missing cert/key files")
	}
}
