// Package crl contient les primitives de gestion de révocation de certificats.
//
// Cette première version reste volontairement simple: elle stocke un set local
// de serials révoqués et expose des points d'extension TODO pour le refresh
// depuis le Control Plane.
package crl

import (
	"crypto/x509"
	"fmt"
	"sync"
)

// Store maintient un set en mémoire de serials révoqués.
type Store struct {
	mu       sync.RWMutex
	revoked  map[string]struct{}
	onRevoke func([]string) // appelé après chaque Replace avec la nouvelle liste
}

// NewStore crée un store CRL vide.
func NewStore() *Store {
	return &Store{revoked: make(map[string]struct{})}
}

// SetOnRevoke enregistre une fonction appelée après chaque remplacement de la CRL.
// fn reçoit la liste complète des serials actuellement révoqués.
// Utilisé par la Gateway pour appeler session.Manager.KillRevoked.
func (s *Store) SetOnRevoke(fn func([]string)) {
	s.mu.Lock()
	s.onRevoke = fn
	s.mu.Unlock()
}

// IsRevoked indique si un serial est actuellement marqué révoqué.
func (s *Store) IsRevoked(serial string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.revoked[serial]
	return ok
}

// Replace remplace atomiquement la CRL en mémoire et déclenche le callback onRevoke.
func (s *Store) Replace(serials []string) {
	next := make(map[string]struct{}, len(serials))
	for _, serial := range serials {
		next[serial] = struct{}{}
	}

	s.mu.Lock()
	s.revoked = next
	cb := s.onRevoke
	s.mu.Unlock()

	// Appeler le callback hors du verrou pour éviter le deadlock
	if cb != nil && len(serials) > 0 {
		cb(serials)
	}
}

// Snapshot retourne une copie du set actuel.
func (s *Store) Snapshot() map[string]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]struct{}, len(s.revoked))
	for k := range s.revoked {
		cp[k] = struct{}{}
	}
	return cp
}

// CertSerial retourne le numéro de série d'un certificat X.509 en hex minuscules,
// dans le même format que celui stocké par le CP: fmt.Sprintf("%x", sn.Bytes()).
func CertSerial(cert *x509.Certificate) string {
	return fmt.Sprintf("%x", cert.SerialNumber.Bytes())
}
