package sshca

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ztna/control-plane/internal/config"
	"github.com/ztna/control-plane/internal/logger"
)

// CA represents an SSH Certificate Authority
type CA struct {
	signer     ssh.Signer
	publicKey  ssh.PublicKey
	validity   time.Duration
	principals []string
	logger     *logger.Logger
}

// CertRequest represents a request for an SSH certificate
type CertRequest struct {
	Username   string
	PublicKey  string
	ValidUntil time.Time
}

// New creates a new SSH CA
func New(cfg config.SSHConfig, log *logger.Logger) (*CA, error) {
	// Load or generate CA key
	signer, err := loadOrGenerateCAKey(cfg.CAKeyPath, log)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA key: %w", err)
	}

	validity, err := cfg.CertValidityDuration()
	if err != nil {
		return nil, fmt.Errorf("invalid cert validity: %w", err)
	}

	return &CA{
		signer:     signer,
		publicKey:  signer.PublicKey(),
		validity:   validity,
		principals: cfg.CertPrincipals,
		logger:     log,
	}, nil
}

// loadOrGenerateCAKey loads an existing CA key or generates a new one
func loadOrGenerateCAKey(path string, log *logger.Logger) (ssh.Signer, error) {
	// Try to load existing key
	keyData, err := os.ReadFile(path)
	if err == nil {
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse existing CA key: %w", err)
		}
		log.Info("Loaded existing SSH CA key", "path", path)
		return signer, nil
	}

	// Generate new key if file doesn't exist
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read CA key: %w", err)
	}

	log.Info("Generating new SSH CA key", "path", path)

	// Generate Ed25519 key pair
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Convert to SSH format
	signer, err := ssh.NewSignerFromKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	// Save private key
	privateKeyPEM := ssh.MarshalAuthorizedKey(signer.PublicKey())
	if err := os.WriteFile(path, encodePrivateKey(privKey), 0600); err != nil {
		return nil, fmt.Errorf("failed to save CA key: %w", err)
	}

	// Save public key
	pubKeyPath := path + ".pub"
	if err := os.WriteFile(pubKeyPath, privateKeyPEM, 0644); err != nil {
		log.Warn("Failed to save CA public key", "error", err)
	}

	log.Info("Generated new SSH CA key", "path", path, "type", "ed25519")
	log.Info("CA public key", "key", string(privateKeyPEM))

	// Print fingerprint
	fp := ssh.FingerprintSHA256(signer.PublicKey())
	log.Info("CA fingerprint", "fingerprint", fp)

	// Save CA public key for TrustedUserCAKeys
	caPublicForSSHD := fmt.Sprintf("cert-authority %s", string(privateKeyPEM))
	caFilePath := path + ".trustedkeys"
	if err := os.WriteFile(caFilePath, []byte(caPublicForSSHD), 0644); err != nil {
		log.Warn("Failed to save trusted CA keys file", "error", err)
	} else {
		log.Info("Saved trusted CA keys file (add to sshd_config: TrustedUserCAKeys)", "path", caFilePath)
	}

	return signer, nil
}

// encodePrivateKey encodes an Ed25519 private key in OpenSSH format
func encodePrivateKey(key ed25519.PrivateKey) []byte {
	// Use MarshalPrivateKey to get the PEM block
	pemBlock, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		return nil
	}
	// Encode the PEM block to bytes
	return pem.EncodeToMemory(pemBlock)
}

// IssueCertificate issues a new SSH certificate
func (ca *CA) IssueCertificate(req CertRequest) (string, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return "", fmt.Errorf("username is required")
	}

	// Parse user's public key
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(req.PublicKey))
	if err != nil {
		return "", fmt.Errorf("invalid public key: %w", err)
	}

	// Calculate validity period
	validAfter := uint64(time.Now().Add(-30 * time.Second).Unix()) // Allow 30s clock skew
	validBefore := uint64(req.ValidUntil.Unix())

	validPrincipals := mergePrincipals(username, ca.principals)

	// Create certificate
	cert := &ssh.Certificate{
		Key:             pubKey,
		Serial:          uint64(time.Now().Unix()),
		CertType:        ssh.UserCert,
		KeyId:           username,
		ValidPrincipals: validPrincipals,
		ValidAfter:      validAfter,
		ValidBefore:     validBefore,
		Permissions: ssh.Permissions{
			Extensions: map[string]string{
				"permit-pty":             "",
				"permit-user-rc":         "",
				"permit-port-forwarding": "",
			},
		},
	}

	// Sign certificate
	if err := cert.SignCert(rand.Reader, ca.signer); err != nil {
		return "", fmt.Errorf("failed to sign certificate: %w", err)
	}

	// Marshal certificate to string
	certBytes := ssh.MarshalAuthorizedKey(cert)

	ca.logger.Info("Issued SSH certificate",
		"username", username,
		"key_id", cert.KeyId,
		"principals", validPrincipals,
		"valid_until", req.ValidUntil.Format(time.RFC3339),
	)

	return string(certBytes), nil
}

func mergePrincipals(username string, configured []string) []string {
	seen := make(map[string]struct{}, len(configured)+1)
	out := make([]string, 0, len(configured)+1)

	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, exists := seen[v]; exists {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	add(username)
	for _, principal := range configured {
		add(principal)
	}

	return out
}

// Fingerprint returns the CA public key fingerprint
func (ca *CA) Fingerprint() string {
	return ssh.FingerprintSHA256(ca.publicKey)
}

// PublicKey returns the CA public key in authorized_keys format
func (ca *CA) PublicKey() string {
	return string(ssh.MarshalAuthorizedKey(ca.publicKey))
}

// DefaultValidUntil returns the default certificate expiration time
func (ca *CA) DefaultValidUntil() time.Time {
	return time.Now().Add(ca.validity)
}
