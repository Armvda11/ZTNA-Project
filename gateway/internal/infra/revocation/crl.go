// Package crl contient les primitives de gestion de révocation de certificats.
//
// Cette première version reste volontairement simple: elle stocke un set local
// de serials révoqués et expose des points d'extension TODO pour le refresh
// depuis le Control Plane.
package crl

import (
	"context"
	"sync"
)

// Store maintient un set en mémoire de serials révoqués.
type Store struct {
	mu      sync.RWMutex
	revoked map[string]struct{}
}

// NewStore crée un store CRL vide.
func NewStore() *Store {
	return &Store{revoked: make(map[string]struct{})}
}

// IsRevoked indique si un serial est actuellement marqué révoqué.
func (s *Store) IsRevoked(serial string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.revoked[serial]
	return ok
}

// Replace remplace atomiquement la CRL en mémoire.
func (s *Store) Replace(serials []string) {
	next := make(map[string]struct{}, len(serials))
	for _, serial := range serials {
		next[serial] = struct{}{}
	}

	s.mu.Lock()
	s.revoked = next
	s.mu.Unlock()
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

// StartAutoRefresh est un hook d'intégration future.
//
// TODO:
// - appeler un endpoint CP exposant la CRL
// - parser la réponse (PEM/DER)
// - convertir les serials
// - appeler Replace
func (s *Store) StartAutoRefresh(ctx context.Context) error {
	_ = ctx
	return nil
}
