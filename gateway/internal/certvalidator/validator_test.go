package certvalidator

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ztna/gateway/internal/config"
	"github.com/ztna/gateway/internal/logger"
)

// Mock ConnMetadata for testing
type mockConnMetadata struct {
	user       string
	remoteAddr net.Addr
}

func (m *mockConnMetadata) User() string          { return m.user }
func (m *mockConnMetadata) SessionID() []byte     { return []byte{} }
func (m *mockConnMetadata) ClientVersion() []byte { return []byte("SSH-2.0-Test") }
func (m *mockConnMetadata) ServerVersion() []byte { return []byte("SSH-2.0-Gateway") }
func (m *mockConnMetadata) RemoteAddr() net.Addr  { return m.remoteAddr }
func (m *mockConnMetadata) LocalAddr() net.Addr   { return &net.TCPAddr{} }

// Helper to generate CA key pair
func generateCAKeyPair(t *testing.T) (ssh.Signer, ssh.PublicKey) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	return signer, signer.PublicKey()
}

// Helper to generate user key pair
func generateUserKeyPair(t *testing.T) (ssh.Signer, ssh.PublicKey) {
	t.Helper()
	return generateCAKeyPair(t)
}

// Helper to create a signed certificate
func createSignedCertificate(t *testing.T, caSigner ssh.Signer, userPubKey ssh.PublicKey, keyID string, principals []string, validDuration time.Duration) *ssh.Certificate {
	t.Helper()

	now := uint64(time.Now().Unix())
	cert := &ssh.Certificate{
		Key:             userPubKey,
		CertType:        ssh.UserCert,
		KeyId:           keyID,
		ValidPrincipals: principals,
		ValidAfter:      now - 60, // Valid from 1 minute ago
		ValidBefore:     now + uint64(validDuration.Seconds()),
	}

	if err := cert.SignCert(rand.Reader, caSigner); err != nil {
		t.Fatalf("Failed to sign certificate: %v", err)
	}

	return cert
}

func TestNew_Success(t *testing.T) {
	// This test requires a running Control Plane - skip if not available
	t.Skip("Requires running Control Plane")
}

func TestValidate_NotACertificate(t *testing.T) {
	// Setup
	caSigner, caPublicKey := generateCAKeyPair(t)
	_ = caSigner

	log := logger.New(config.LoggingConfig{Level: "error", Format: "json"})

	validator := &Validator{
		caPublicKey: caPublicKey,
		logger:      log,
	}

	// Use regular public key (not a certificate)
	_, regularKey := generateUserKeyPair(t)

	conn := &mockConnMetadata{
		user:       "alice",
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("10.10.10.10"), Port: 12345},
	}

	// Validate
	result, err := validator.Validate(regularKey, conn)

	// Assert
	if err == nil {
		t.Error("Expected error for non-certificate key")
	}
	if result.Valid {
		t.Error("Expected Valid to be false")
	}
	if result.Error == "" {
		t.Error("Expected Error field to be set")
	}
}

func TestValidate_ValidCertificate(t *testing.T) {
	// Setup
	caSigner, caPublicKey := generateCAKeyPair(t)
	_, userPubKey := generateUserKeyPair(t)

	// Create valid certificate with username as principal (for SSH authentication)
	// In ZTNA, the certificate principals are the usernames allowed to authenticate,
	// NOT the resource names. Resource authorization happens via policy check.
	cert := createSignedCertificate(t, caSigner, userPubKey, "alice", []string{"alice"}, 15*time.Minute)

	log := logger.New(config.LoggingConfig{Level: "info", Format: "json"})

	validator := &Validator{
		caPublicKey: caPublicKey,
		logger:      log,
	}

	conn := &mockConnMetadata{
		user:       "alice",
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("10.10.10.10"), Port: 12345},
	}

	// Validate
	result, err := validator.Validate(cert, conn)

	// Assert
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !result.Valid {
		t.Errorf("Expected Valid to be true, got error: %s", result.Error)
	}
	if result.Username != "alice" {
		t.Errorf("Expected username 'alice', got: %s", result.Username)
	}
	if len(result.Principals) != 1 {
		t.Errorf("Expected 1 principal, got: %d", len(result.Principals))
	}
	if len(result.Principals) > 0 && result.Principals[0] != "alice" {
		t.Errorf("Expected principal 'alice', got: %s", result.Principals[0])
	}
	if result.KeyID != "alice" {
		t.Errorf("Expected key_id 'alice', got: %s", result.KeyID)
	}
}

func TestValidate_ExpiredCertificate(t *testing.T) {
	// Setup
	caSigner, caPublicKey := generateCAKeyPair(t)
	_, userPubKey := generateUserKeyPair(t)

	// Create expired certificate
	cert := createSignedCertificate(t, caSigner, userPubKey, "bob", []string{"bob"}, -5*time.Second)

	log := logger.New(config.LoggingConfig{Level: "error", Format: "json"})

	validator := &Validator{
		caPublicKey: caPublicKey,
		logger:      log,
	}

	conn := &mockConnMetadata{
		user:       "bob",
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("10.10.10.11"), Port: 22222},
	}

	// Validate
	result, err := validator.Validate(cert, conn)

	// Assert
	if err == nil {
		t.Error("Expected error for expired certificate")
	}
	if result.Valid {
		t.Error("Expected Valid to be false")
	}
	// Error can be either from our validation or from CheckCert
	if result.Error == "" {
		t.Error("Expected error for expired certificate")
	}
}

func TestValidate_WrongCA(t *testing.T) {
	// Setup
	_, caPublicKey := generateCAKeyPair(t)
	wrongCASigner, wrongCAPublicKey := generateCAKeyPair(t)
	_, userPubKey := generateUserKeyPair(t)

	// Certificate signed by wrong CA
	cert := createSignedCertificate(t, wrongCASigner, userPubKey, "charlie", []string{"charlie"}, 15*time.Minute)

	log := logger.New(config.LoggingConfig{Level: "error", Format: "json"})

	// Validator configured with different CA
	validator := &Validator{
		caPublicKey: caPublicKey, // Use the CORRECT CA, not the wrong one
		logger:      log,
	}
	_ = wrongCAPublicKey

	conn := &mockConnMetadata{
		user:       "charlie",
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("10.10.10.12"), Port: 33333},
	}

	// Validate
	result, err := validator.Validate(cert, conn)

	// Assert
	if err == nil {
		t.Error("Expected error for certificate signed by wrong CA")
	}
	if result.Valid {
		t.Error("Expected Valid to be false")
	}
}

func TestValidate_RejectsMismatchedSSHUserAndKeyID(t *testing.T) {
	// Setup
	caSigner, caPublicKey := generateCAKeyPair(t)
	_, userPubKey := generateUserKeyPair(t)

	// Certificate with key_id "alice"
	cert := createSignedCertificate(t, caSigner, userPubKey, "alice", []string{"ztna-user"}, 15*time.Minute)

	log := logger.New(config.LoggingConfig{Level: "error", Format: "json"})

	validator := &Validator{
		caPublicKey: caPublicKey,
		logger:      log,
	}

	// Connection with different username "bob"
	conn := &mockConnMetadata{
		user:       "bob",
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("10.10.10.13"), Port: 44444},
	}

	// Validate
	result, err := validator.Validate(cert, conn)

	// Assert - mismatch must be rejected
	if err == nil {
		t.Errorf("Expected error when ssh user and key_id differ")
	}
	if result.Valid {
		t.Errorf("Expected Valid to be false when ssh user and key_id differ")
	}
}

func TestGetCAFingerprint(t *testing.T) {
	_, caPublicKey := generateCAKeyPair(t)
	log := logger.New(config.LoggingConfig{Level: "error", Format: "json"})

	validator := &Validator{
		caPublicKey: caPublicKey,
		logger:      log,
	}

	fingerprint := validator.GetCAFingerprint()

	// Should be SHA256 fingerprint format
	if len(fingerprint) < 40 {
		t.Errorf("Fingerprint too short: %s", fingerprint)
	}

	// Should start with "SHA256:"
	if fingerprint[:7] != "SHA256:" {
		t.Errorf("Expected fingerprint to start with 'SHA256:', got: %s", fingerprint)
	}

	fmt.Printf("CA Fingerprint: %s\n", fingerprint)
}
