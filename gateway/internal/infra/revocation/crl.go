// Package crl gère la révocation de certificats en intégrant la CRL
// publiée par le Control Plane.
//
// Fonctionnement :
//  1. Le store maintient un set en mémoire des serials révoqués.
//  2. StartAutoRefresh récupère périodiquement la CRL depuis le CP
//     (GET /pki/device-ca/crl) et met à jour le set local.
//  3. Le listener mTLS consulte IsRevoked() après chaque handshake TLS
//     pour rejeter les certificats révoqués.
package crl

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Store maintient un set en mémoire de serials révoqués.
type Store struct {
	mu        sync.RWMutex
	revoked   map[string]struct{}
	lastFetch time.Time
	log       *slog.Logger
	crlURL    string
	client    *http.Client
	interval  time.Duration
}

// NewStore crée un store CRL vide.
func NewStore() *Store {
	return &Store{
		revoked: make(map[string]struct{}),
		log:     slog.Default(),
	}
}

// NewStoreWithConfig crée un store CRL configuré pour le refresh automatique.
func NewStoreWithConfig(crlURL string, interval time.Duration, client *http.Client, log *slog.Logger) *Store {
	return &Store{
		revoked:  make(map[string]struct{}),
		crlURL:   crlURL,
		interval: interval,
		client:   client,
		log:      log,
	}
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

// Count retourne le nombre de serials révoqués.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.revoked)
}

// LastFetch retourne l'heure du dernier refresh réussi.
func (s *Store) LastFetch() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastFetch
}

// StartAutoRefresh télécharge la CRL depuis le CP à intervalle régulier.
// Bloque jusqu'à l'annulation du contexte. Le premier fetch est immédiat.
func (s *Store) StartAutoRefresh(ctx context.Context) error {
	if s.crlURL == "" {
		s.log.Warn("CRL auto-refresh désactivé: pas d'URL configurée")
		return nil
	}
	if s.client == nil {
		s.log.Warn("CRL auto-refresh désactivé: pas de client HTTP")
		return nil
	}

	interval := s.interval
	if interval <= 0 {
		interval = 60 * time.Second
	}

	s.log.Info("démarrage CRL auto-refresh",
		"url", s.crlURL,
		"interval", interval.String(),
	)

	// Premier fetch immédiat
	if err := s.fetchAndUpdate(ctx); err != nil {
		s.log.Warn("premier fetch CRL échoué", "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("CRL auto-refresh arrêté")
			return nil
		case <-ticker.C:
			if err := s.fetchAndUpdate(ctx); err != nil {
				s.log.Warn("refresh CRL échoué", "error", err)
			}
		}
	}
}

// fetchAndUpdate télécharge la CRL PEM depuis le CP et met à jour le store.
func (s *Store) fetchAndUpdate(ctx context.Context) error {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", s.crlURL, nil)
	if err != nil {
		return fmt.Errorf("création requête CRL: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("téléchargement CRL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CRL endpoint retourne status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB max
	if err != nil {
		return fmt.Errorf("lecture body CRL: %w", err)
	}

	serials, err := parseCRLSerials(body)
	if err != nil {
		return fmt.Errorf("parsing CRL: %w", err)
	}

	s.Replace(serials)
	s.mu.Lock()
	s.lastFetch = time.Now()
	s.mu.Unlock()

	s.log.Info("CRL mise à jour",
		"revoked_count", len(serials),
		"url", s.crlURL,
	)

	return nil
}

// parseCRLSerials extrait les serial numbers depuis une CRL au format PEM ou DER.
func parseCRLSerials(data []byte) ([]string, error) {
	var derBytes []byte

	// Essayer de décoder comme PEM
	block, _ := pem.Decode(data)
	if block != nil && block.Type == "X509 CRL" {
		derBytes = block.Bytes
	} else {
		// Essayer comme DER brut
		derBytes = data
	}

	crl, err := x509.ParseRevocationList(derBytes)
	if err != nil {
		return nil, fmt.Errorf("parse CRL: %w", err)
	}

	serials := make([]string, 0, len(crl.RevokedCertificateEntries))
	for _, entry := range crl.RevokedCertificateEntries {
		serials = append(serials, entry.SerialNumber.String())
	}

	return serials, nil
}
