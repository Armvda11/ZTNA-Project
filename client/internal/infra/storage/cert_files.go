// Package storage fournit les mécanismes de persistance locale côté client.
//
// Ce fichier centralise la lecture/écriture des artefacts mTLS.
package storage

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"client/internal/config"
	"client/internal/core/domain"
)

const (
	clientCertFileName = "client.crt"
	clientKeyFileName  = "client.key"
	clientCAFileName   = "client-ca.crt"
)

// CertFileStore stocke les certificats/clé privée mTLS dans storage.path.
type CertFileStore struct {
	cfg *config.Config
	log *slog.Logger
}

// NewCertFileStore crée un store de certificats basé sur le système de fichiers.
func NewCertFileStore(cfg *config.Config, log *slog.Logger) *CertFileStore {
	return &CertFileStore{cfg: cfg, log: log}
}

// SaveCertAndKey sauvegarde cert + key avec permissions restrictives.
//
// TODO: Ajouter une écriture atomique (fichier temporaire + rename)
// pour éviter les corruptions en cas de crash durant la sauvegarde.
func (s *CertFileStore) SaveCertAndKey(certPEM, keyPEM []byte) error {
	if err := os.MkdirAll(s.cfg.Storage.Path, 0700); err != nil {
		return fmt.Errorf("impossible de créer le dossier de stockage: %w", err)
	}

	certPath := filepath.Join(s.cfg.Storage.Path, clientCertFileName)
	keyPath := filepath.Join(s.cfg.Storage.Path, clientKeyFileName)

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("impossible d'écrire le certificat: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("impossible d'écrire la clé privée: %w", err)
	}

	s.log.Debug("certificat et clé privée sauvegardés", "cert", certPath, "key", keyPath)
	return nil
}

// SaveCACert sauvegarde le certificat de la CA émettrice.
func (s *CertFileStore) SaveCACert(caCertPEM []byte) error {
	if len(caCertPEM) == 0 {
		return nil
	}
	if err := os.MkdirAll(s.cfg.Storage.Path, 0700); err != nil {
		return fmt.Errorf("impossible de créer le dossier de stockage: %w", err)
	}
	caPath := filepath.Join(s.cfg.Storage.Path, clientCAFileName)
	if err := os.WriteFile(caPath, caCertPEM, 0644); err != nil {
		return fmt.Errorf("impossible d'écrire le certificat CA: %w", err)
	}
	s.log.Debug("certificat CA sauvegardé", "path", caPath)
	return nil
}

// LoadCertAndKey charge cert + key depuis storage.path.
func (s *CertFileStore) LoadCertAndKey() (certPEM, keyPEM []byte, err error) {
	certPath := filepath.Join(s.cfg.Storage.Path, clientCertFileName)
	keyPath := filepath.Join(s.cfg.Storage.Path, clientKeyFileName)

	certPEM, err = os.ReadFile(certPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, domain.ErrNoCertificate
		}
		return nil, nil, fmt.Errorf("impossible de lire le certificat: %w", err)
	}

	keyPEM, err = os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, domain.ErrNoCertificate
		}
		return nil, nil, fmt.Errorf("impossible de lire la clé privée: %w", err)
	}

	// Validation cert/clé + expiration déléguée à credentials.Client.LoadCertAndKey()
	return certPEM, keyPEM, nil
}
