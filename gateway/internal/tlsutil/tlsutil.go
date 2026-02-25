// Package tlsutil provides TLS helpers for the ZTNA gateway.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

// LoadOrGenerateSelfSignedCert loads server TLS cert from disk, or generates
// an ephemeral self-signed ECDSA P-256 cert when paths are not configured.
func LoadOrGenerateSelfSignedCert(certFile, keyFile string) (tls.Certificate, error) {
	if certFile != "" && keyFile != "" {
		if _, err := os.Stat(certFile); err == nil {
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return tls.Certificate{}, fmt.Errorf("load server cert: %w", err)
			}
			return cert, nil
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate server key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "ztna-gateway"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create self-signed cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if certFile != "" && keyFile != "" {
		_ = os.WriteFile(certFile, certPEM, 0o600)
		_ = os.WriteFile(keyFile, keyPEM, 0o600)
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// FetchDeviceCACert fetches the Device CA certificate PEM from the CP PKI endpoint.
func FetchDeviceCACert(cpURL string, httpClient *http.Client) ([]byte, error) {
	resp, err := httpClient.Get(cpURL + "/pki/device-ca/cert")
	if err != nil {
		return nil, fmt.Errorf("fetch device ca cert: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch device ca cert: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read device ca cert body: %w", err)
	}
	return data, nil
}

// BuildClientCertPool builds an x509.CertPool from PEM-encoded certificate data.
func BuildClientCertPool(pemData []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("no certificates found in PEM data")
	}
	return pool, nil
}

// LoadDeviceCACertFromFile loads a PEM certificate file.
func LoadDeviceCACertFromFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read device ca cert %s: %w", path, err)
	}
	return data, nil
}

// LoadCertPoolFromFile creates an x509.CertPool from a PEM file.
func LoadCertPoolFromFile(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cert pool file %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("parse cert pool file %s: no PEM cert found", path)
	}
	return pool, nil
}

// CertFingerprintSHA256 returns a colon-separated SHA256 fingerprint for logging/registration.
func CertFingerprintSHA256(der []byte) string {
	sum := sha256.Sum256(der)
	parts := make([]string, 0, len(sum))
	for _, b := range sum {
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	return strings.Join(parts, ":")
}
