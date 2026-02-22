package deviceca

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
)

func generateTestCSR(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	template := &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "test-client"},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		t.Fatalf("create CSR: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
}

func parseCACert(t *testing.T, pemData []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(pemData)
	if block == nil {
		t.Fatal("cannot decode PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}

func TestLoadOrCreate_CreatesNewCA(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "ca.key")
	certPath := filepath.Join(dir, "ca.crt")

	ca, err := LoadOrCreate(keyPath, certPath)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if ca == nil {
		t.Fatal("expected non-nil CA")
	}

	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("CA key not created: %v", err)
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("CA cert not created: %v", err)
	}

	certPEM := ca.CACertPEM()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("CA cert PEM is invalid")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	if !cert.IsCA {
		t.Error("CA cert should have IsCA=true")
	}
}

func TestLoadOrCreate_LoadsExisting(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "ca.key")
	certPath := filepath.Join(dir, "ca.crt")

	ca1, err := LoadOrCreate(keyPath, certPath)
	if err != nil {
		t.Fatalf("first LoadOrCreate: %v", err)
	}
	ca2, err := LoadOrCreate(keyPath, certPath)
	if err != nil {
		t.Fatalf("second LoadOrCreate: %v", err)
	}
	cert1 := parseCACert(t, ca1.CACertPEM())
	cert2 := parseCACert(t, ca2.CACertPEM())
	if cert1.SerialNumber.Cmp(cert2.SerialNumber) != 0 {
		t.Error("second load returned a different serial — CA was regenerated unexpectedly")
	}
}

func TestSignClientCSR_ProducesValidCert(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreate(filepath.Join(dir, "ca.key"), filepath.Join(dir, "ca.crt"))
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}

	csrPEM := generateTestCSR(t)
	certPEM, fingerprint, err := ca.SignClientCSR(
csrPEM,
"alice",
"sub-abc-123",
[]string{"admins", "devs"},
24*time.Hour,
[]string{"rsa", "ecdsa"},
)
	if err != nil {
		t.Fatalf("SignClientCSR: %v", err)
	}
	if len(fingerprint) == 0 {
		t.Error("fingerprint must not be empty")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("cert PEM is invalid")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	if cert.Subject.CommonName != "alice" {
		t.Errorf("CN: want alice, got %s", cert.Subject.CommonName)
	}
	if cert.Subject.SerialNumber != "sub-abc-123" {
		t.Errorf("SerialNumber: want sub-abc-123, got %s", cert.Subject.SerialNumber)
	}
	if len(cert.Subject.Organization) < 2 {
		t.Errorf("Org: expected at least 2 entries (admins, devs), got %v", cert.Subject.Organization)
	} else {
		orgSet := make(map[string]bool)
		for _, o := range cert.Subject.Organization {
			orgSet[o] = true
		}
		if !orgSet["admins"] || !orgSet["devs"] {
			t.Errorf("Org: expected admins and devs, got %v", cert.Subject.Organization)
		}
	}

	pool := x509.NewCertPool()
	pool.AddCert(parseCACert(t, ca.CACertPEM()))
	if _, err := cert.Verify(x509.VerifyOptions{
Roots:     pool,
KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
}); err != nil {
		t.Errorf("cert verification against CA failed: %v", err)
	}
}

func TestSignClientCSR_RejectsUnknownKeyType(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreate(filepath.Join(dir, "ca.key"), filepath.Join(dir, "ca.crt"))
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}

	csrPEM := generateTestCSR(t)
	_, _, err = ca.SignClientCSR(
csrPEM,
"alice",
"sub-1",
nil,
24*time.Hour,
[]string{"ecdsa"},
)
	if err == nil {
		t.Fatal("expected error for disallowed RSA key, got nil")
	}
}

func TestGenerateCRL(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreate(filepath.Join(dir, "ca.key"), filepath.Join(dir, "ca.crt"))
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}

	revoked := []pkix.RevokedCertificate{
		{SerialNumber: big.NewInt(42), RevocationTime: time.Now()},
	}
	crlDER, err := ca.GenerateCRL(revoked)
	if err != nil {
		t.Fatalf("GenerateCRL: %v", err)
	}
	if len(crlDER) == 0 {
		t.Error("CRL must not be empty")
	}
	if _, err := x509.ParseRevocationList(crlDER); err != nil {
		t.Errorf("invalid CRL DER: %v", err)
	}
}
