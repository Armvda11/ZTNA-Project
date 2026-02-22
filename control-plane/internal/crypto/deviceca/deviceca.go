// Package deviceca implements a minimal X.509 CA for issuing short-lived
// client TLS certificates used for mTLS between ZTNA clients and gateways.
//
// Subject conventions used when signing CSRs:
//
//	CommonName         = ZTNA username
//	SerialNumber       = ZTNA subject UUID (sub claim)
//	Organization       = ZTNA groups (one slice entry per group)
//
// The gateway can reconstruct model.Subject from these fields without any
// custom OID or OIDC introspection.
package deviceca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// pemTypeKey holds the PEM block type for ECDSA private keys.
	pemTypeKey  = "EC PRIVATE KEY"
	pemTypeCert = "CERTIFICATE"
)

// CA is a loaded X.509 certificate authority.
type CA struct {
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
	rawCert []byte // DER-encoded CA cert for fast CACertPEM()
}

// LoadOrCreate loads the CA key+cert from disk. If either file is absent the
// CA is freshly generated (ECDSA P-256) and both files are written.
func LoadOrCreate(keyPath, certPath string) (*CA, error) {
	keyData, keyErr := os.ReadFile(keyPath)
	certData, certErr := os.ReadFile(certPath)

	if keyErr == nil && certErr == nil {
		// Both files exist: parse and return.
		key, err := parseECKey(keyData)
		if err != nil {
			return nil, fmt.Errorf("device ca: parse key: %w", err)
		}
		cert, raw, err := parseCert(certData)
		if err != nil {
			return nil, fmt.Errorf("device ca: parse cert: %w", err)
		}
		return &CA{cert: cert, key: key, rawCert: raw}, nil
	}

	// At least one file missing: generate a fresh CA.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("device ca: generate key: %w", err)
	}

	serial, err := randSerial()
	if err != nil {
		return nil, fmt.Errorf("device ca: random serial: %w", err)
	}

	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "ZTNA Device CA",
			Organization: []string{"ZTNA"},
		},
		NotBefore:             now.Add(-30 * time.Second),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour), // 10 years
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
	}

	raw, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("device ca: create self-signed cert: %w", err)
	}

	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		return nil, fmt.Errorf("device ca: parse generated cert: %w", err)
	}

	// Persist key and cert.
	if err := mkdirAll(keyPath); err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("device ca: marshal key: %w", err)
	}
	if err := writeFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: pemTypeKey, Bytes: keyDER}), 0o600); err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: pemTypeCert, Bytes: raw})
	if err := writeFile(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}

	return &CA{cert: cert, key: key, rawCert: raw}, nil
}

// CACertPEM returns the PEM-encoded CA certificate for distribution to
// gateways (used as the ClientCAs trust pool).
func (c *CA) CACertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: pemTypeCert, Bytes: c.rawCert})
}

// SignClientCSR validates and signs a PEM-encoded PKCS#10 CSR. The returned
// certificate is PEM-encoded and valid for ttl. The allowedKeyTypes slice
// restricts accepted key algorithms ("ed25519", "ecdsa-p256"); pass nil to
// accept any key type.
//
// Subject in the issued cert is set to:
//
//	CommonName    = username    (from caller, not from CSR)
//	SerialNumber  = sub         (OIDC sub UUID)
//	Organization  = groups
//
// The CSR is only used for its public key; all other fields are ignored.
func (c *CA) SignClientCSR(
	csrPEM []byte,
	username, sub string,
	groups []string,
	ttl time.Duration,
	allowedKeyTypes []string,
) (certPEM []byte, fingerprint string, err error) {
	// Parse CSR.
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, "", fmt.Errorf("device ca: invalid CSR PEM")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("device ca: parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, "", fmt.Errorf("device ca: invalid CSR signature: %w", err)
	}

	// Validate key type.
	if err := checkKeyType(csr.PublicKey, allowedKeyTypes); err != nil {
		return nil, "", err
	}

	serial, err := randSerial()
	if err != nil {
		return nil, "", fmt.Errorf("device ca: random serial: %w", err)
	}

	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   username,
			SerialNumber: sub,
			Organization: groups,
		},
		NotBefore:             now.Add(-30 * time.Second),
		NotAfter:              now.Add(ttl),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	raw, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, csr.PublicKey, c.key)
	if err != nil {
		return nil, "", fmt.Errorf("device ca: sign CSR: %w", err)
	}

	fp := certFingerprint(raw)
	return pem.EncodeToMemory(&pem.Block{Type: pemTypeCert, Bytes: raw}), fp, nil
}

// SerialHex returns the CA certificate serial as a hex string.
func (c *CA) SerialHex() string {
	return hex.EncodeToString(c.cert.SerialNumber.Bytes())
}

// GenerateCRL produces a DER-encoded Certificate Revocation List containing
// all provided revoked certificate entries.
func (c *CA) GenerateCRL(revoked []pkix.RevokedCertificate) ([]byte, error) {
	now := time.Now().UTC()
	rl := &x509.RevocationList{
		Number:              big.NewInt(now.UnixNano()),
		ThisUpdate:          now,
		NextUpdate:          now.Add(24 * time.Hour),
		RevokedCertificates: revoked,
	}

	crlDER, err := x509.CreateRevocationList(rand.Reader, rl, c.cert, c.key)
	if err != nil {
		return nil, fmt.Errorf("device ca: create CRL: %w", err)
	}
	return crlDER, nil
}

// ──────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────

func certFingerprint(rawDER []byte) string {
	h := sha256.Sum256(rawDER)
	parts := make([]string, len(h))
	for i, b := range h {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(parts, ":")
}

func randSerial() (*big.Int, error) {
	max := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, max)
}

func checkKeyType(pub any, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	var got string
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		switch k.Curve {
		case elliptic.P256():
			got = "ecdsa-p256"
		default:
			got = "ecdsa-other"
		}
	case *rsa.PublicKey:
		got = "rsa"
	default:
		got = fmt.Sprintf("%T", pub)
	}

	// ed25519.PublicKey is a []byte under the hood; handle via type string.
	if strings.Contains(fmt.Sprintf("%T", pub), "ed25519") {
		got = "ed25519"
	}

	// Accept "ecdsa" as an alias that matches any ecdsa-* variant.
	for _, a := range allowed {
		if strings.EqualFold(a, got) {
			return nil
		}
		if strings.EqualFold(a, "ecdsa") && strings.HasPrefix(got, "ecdsa") {
			return nil
		}
	}
	return fmt.Errorf("device ca: key type %q not allowed (allowed: %v)", got, allowed)
}

func parseECKey(pemData []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func parseCert(pemData []byte) (*x509.Certificate, []byte, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, nil, fmt.Errorf("no PEM block found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, block.Bytes, nil
}

func mkdirAll(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("device ca: mkdir %s: %w", dir, err)
	}
	return nil
}

func writeFile(path string, data []byte, perm os.FileMode) error {
	if err := mkdirAll(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("device ca: write %s: %w", path, err)
	}
	return nil
}
