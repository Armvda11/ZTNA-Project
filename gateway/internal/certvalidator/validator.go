package certvalidator

import (
	"crypto/subtle"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ztna/gateway/internal/controlplane"
	"github.com/ztna/gateway/internal/logger"
)

// Validator validates SSH certificates signed by the Control Plane CA
type Validator struct {
	caPublicKey ssh.PublicKey
	cpClient    *controlplane.Client
	logger      *logger.Logger
}

// New creates a new certificate validator
func New(cpClient *controlplane.Client, caEndpoint string, log *logger.Logger) (*Validator, error) {
	// Fetch CA public key from Control Plane
	log.Info("Fetching CA public key from Control Plane", "endpoint", caEndpoint)
	
	caKeyString, err := cpClient.GetCAPublicKey(caEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get CA public key: %w", err)
	}

	// Parse CA public key
	caPublicKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(caKeyString))
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA public key: %w", err)
	}

	log.Info("CA public key loaded successfully", "key_type", caPublicKey.Type())

	return &Validator{
		caPublicKey: caPublicKey,
		cpClient:    cpClient,
		logger:      log,
	}, nil
}

// ValidateResult contains the validation result
type ValidateResult struct {
	Valid       bool
	Username    string
	Principals  []string
	ValidBefore time.Time
	ValidAfter  time.Time
	KeyID       string
	Error       string
}

// Validate validates an SSH certificate
func (v *Validator) Validate(key ssh.PublicKey, connMetadata ssh.ConnMetadata) (*ValidateResult, error) {
	result := &ValidateResult{
		Valid: false,
	}

	// 1. Check if the key is actually a certificate
	cert, ok := key.(*ssh.Certificate)
	if !ok {
		result.Error = "authentication key is not an SSH certificate"
		v.logger.Warn("Certificate validation failed: not a certificate",
			"user", connMetadata.User(),
			"remote", connMetadata.RemoteAddr(),
			"key_type", key.Type())
		return result, fmt.Errorf("%s", result.Error)
	}

	// 2. Verify certificate signature with CA public key
	// First check if the certificate was signed by our CA
	if cert.SignatureKey == nil {
		result.Error = "certificate has no signature key"
		v.logger.Warn("Certificate validation failed: no signature key",
			"user", connMetadata.User(),
			"key_id", cert.KeyId)
		return result, fmt.Errorf("%s", result.Error)
	}

	if cert.CertType != ssh.UserCert {
		result.Error = "certificate is not a user certificate"
		v.logger.Warn("Certificate validation failed: wrong cert type",
			"user", connMetadata.User(),
			"key_id", cert.KeyId,
			"cert_type", cert.CertType)
		return result, fmt.Errorf("%s", result.Error)
	}

	if cert.KeyId == "" {
		result.Error = "certificate key_id is empty"
		v.logger.Warn("Certificate validation failed: empty key_id",
			"user", connMetadata.User())
		return result, fmt.Errorf("%s", result.Error)
	}

	// Compare signature key with our CA public key
	caBytes := v.caPublicKey.Marshal()
	sigKeyBytes := cert.SignatureKey.Marshal()
	if subtle.ConstantTimeCompare(caBytes, sigKeyBytes) != 1 {
		result.Error = "certificate not signed by trusted CA"
		v.logger.Warn("Certificate validation failed: wrong CA",
			"user", connMetadata.User(),
			"key_id", cert.KeyId,
			"ca_fingerprint", ssh.FingerprintSHA256(v.caPublicKey),
			"cert_ca_fingerprint", ssh.FingerprintSHA256(cert.SignatureKey))
		return result, fmt.Errorf("%s", result.Error)
	}

	// Now validate the certificate signature and time bounds
	certChecker := &ssh.CertChecker{
		IsUserAuthority: func(auth ssh.PublicKey) bool {
			// Always return true since we already verified above
			return true
		},
	}

	principal := ""
	if len(cert.ValidPrincipals) > 0 {
		principal = cert.ValidPrincipals[0]
	}
	if principal == "" {
		errMsg := "certificate has no valid principals"
		result.Error = errMsg
		v.logger.Warn("Certificate validation failed: no principals",
			"user", connMetadata.User(),
			"key_id", cert.KeyId)
		return result, fmt.Errorf("%s", errMsg)
	}

	if err := certChecker.CheckCert(principal, cert); err != nil {
		errMsg := fmt.Sprintf("certificate check failed: %v", err)
		result.Error = errMsg
		v.logger.Warn("Certificate validation failed",
			"user", connMetadata.User(),
			"remote", connMetadata.RemoteAddr(),
			"key_id", cert.KeyId,
			"error", err)
		return result, fmt.Errorf("%s", errMsg)
	}

	// 3. Verify certificate is not expired
	now := time.Now()
	validAfter := time.Unix(int64(cert.ValidAfter), 0)
	validBefore := time.Unix(int64(cert.ValidBefore), 0)

	if now.Before(validAfter) {
		errMsg := fmt.Sprintf("certificate not yet valid (valid after %s)", validAfter)
		result.Error = errMsg
		v.logger.Warn("Certificate not yet valid",
			"user", connMetadata.User(),
			"key_id", cert.KeyId,
			"valid_after", validAfter)
		return result, fmt.Errorf("%s", errMsg)
	}

	if now.After(validBefore) {
		errMsg := fmt.Sprintf("certificate expired (valid until %s)", validBefore)
		result.Error = errMsg
		v.logger.Warn("Certificate expired",
			"user", connMetadata.User(),
			"key_id", cert.KeyId,
			"valid_before", validBefore,
			"expired_since", now.Sub(validBefore))
		return result, fmt.Errorf("%s", errMsg)
	}

	// 4. Extract identity and principals
	result.Valid = true
	result.Username = cert.KeyId
	result.Principals = cert.ValidPrincipals
	result.ValidBefore = validBefore
	result.ValidAfter = validAfter
	result.KeyID = cert.KeyId

	v.logger.Info("Certificate validated successfully",
		"user", connMetadata.User(),
		"key_id", cert.KeyId,
		"principals", cert.ValidPrincipals,
		"valid_until", validBefore,
		"remaining", validBefore.Sub(now))

	return result, nil
}

// GetCAPublicKey returns the CA public key (for testing/debugging)
func (v *Validator) GetCAPublicKey() ssh.PublicKey {
	return v.caPublicKey
}

// GetCAFingerprint returns the CA public key fingerprint
func (v *Validator) GetCAFingerprint() string {
	return ssh.FingerprintSHA256(v.caPublicKey)
}
