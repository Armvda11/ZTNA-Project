package sshca

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

type CA struct {
	signer ssh.Signer
}

func LoadOrCreate(keyPath string) (*CA, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read ssh ca key: %w", err)
		}

		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate ssh ca key: %w", err)
		}

		pemBytes, err := marshalEd25519PrivateKey(priv)
		if err != nil {
			return nil, fmt.Errorf("marshal ssh ca key: %w", err)
		}

		if err := os.WriteFile(keyPath, pemBytes, 0o600); err != nil {
			return nil, fmt.Errorf("write ssh ca key: %w", err)
		}
		keyData = pemBytes
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parse ssh ca key: %w", err)
	}

	return &CA{signer: signer}, nil
}

func (c *CA) SignUserCert(publicKey ssh.PublicKey, principals []string, ttl time.Duration, keyID string) (string, time.Time, error) {
	now := time.Now().UTC()
	if keyID == "" && len(principals) > 0 {
		keyID = principals[0]
	}
	cert := &ssh.Certificate{
		Key:             publicKey,
		Serial:          uint64(now.UnixNano()),
		CertType:        ssh.UserCert,
		KeyId:           keyID,
		ValidPrincipals: principals,
		ValidAfter:      uint64(now.Add(-30 * time.Second).Unix()),
		ValidBefore:     uint64(now.Add(ttl).Unix()),
	}

	if err := cert.SignCert(rand.Reader, c.signer); err != nil {
		return "", time.Time{}, fmt.Errorf("sign ssh cert: %w", err)
	}

	return string(ssh.MarshalAuthorizedKey(cert)), time.Unix(int64(cert.ValidBefore), 0).UTC(), nil
}

// CAPubKeyAuthorizedKey returns the SSH CA public key in authorized_keys format,
// suitable for use as a TrustedUserCAKeys entry on SSH servers.
func (c *CA) CAPubKeyAuthorizedKey() []byte {
	return ssh.MarshalAuthorizedKey(c.signer.PublicKey())
}

func marshalEd25519PrivateKey(key ed25519.PrivateKey) ([]byte, error) {
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}

	block := &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}
	return pem.EncodeToMemory(block), nil
}
