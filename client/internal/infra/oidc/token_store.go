// Package oidc — token_store.go
//
// TokenStore gère le stockage sécurisé des tokens OIDC (access_token,
// refresh_token, id_token) sur le système de fichiers local.
//
// Stratégie de sécurité (par ordre de priorité, TODO) :
//   - Windows : DPAPI (Data Protection API) pour chiffrer les données au repos
//   - Linux   : libsecret / GNOME Keyring / KDE Wallet via D-Bus
//   - macOS   : Keychain Access via Security framework
//   - Fallback: fichier chiffré AES-256-GCM avec clé dérivée (PBKDF2)
//
// Pour le lab, un stockage fichier simple (JSON non chiffré) est utilisé
// comme placeholder. NE PAS utiliser en production sans chiffrement.
package oidc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const tokenFileName = "tokens.json"

// TokenStore gère la persistance des tokens OIDC sur le disque.
type TokenStore struct {
	storagePath string
	log         *slog.Logger
}

// NewTokenStore crée un TokenStore qui sauvegarde les tokens dans le
// répertoire indiqué.
func NewTokenStore(storagePath string, log *slog.Logger) *TokenStore {
	return &TokenStore{
		storagePath: storagePath,
		log:         log,
	}
}

// Save persiste le TokenSet sur le disque.
//
// TODO: Chiffrer les données avant écriture :
//   - Détecter l'OS et utiliser le mécanisme approprié (DPAPI, Keyring, etc.)
//   - Fallback : AES-256-GCM avec clé dérivée de la phrase de passe utilisateur
//   - Définir les permissions fichier strictes (0600)
//
// TODO: Protéger contre les écritures concurrentes (file lock)
func (s *TokenStore) Save(tokens *TokenSet) error {
	s.log.Debug("sauvegarde des tokens OIDC", "path", s.storagePath)

	if err := os.MkdirAll(s.storagePath, 0700); err != nil {
		return fmt.Errorf("impossible de créer le répertoire de stockage: %w", err)
	}

	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("impossible de sérialiser les tokens: %w", err)
	}

	tokenPath := filepath.Join(s.storagePath, tokenFileName)

	// TODO: chiffrer data avant écriture
	// WARNING: stockage en clair — lab uniquement
	if err := os.WriteFile(tokenPath, data, 0600); err != nil {
		return fmt.Errorf("impossible d'écrire les tokens: %w", err)
	}

	s.log.Debug("tokens sauvegardés avec succès")
	return nil
}

// Load charge le TokenSet depuis le disque.
//	
// TODO: Déchiffrer les données après lecture (symétrique avec Save)
// TODO: Valider l'intégrité des données (HMAC ou tag GCM)
func (s *TokenStore) Load() (*TokenSet, error) {
	tokenPath := filepath.Join(s.storagePath, tokenFileName)

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("aucun token stocké ; exécutez 'ztna login' d'abord")
		}
		return nil, fmt.Errorf("impossible de lire les tokens: %w", err)
	}

	// TODO: déchiffrer data avant désérialisation
	var tokens TokenSet
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("impossible de parser les tokens stockés: %w", err)
	}

	return &tokens, nil
}

// Delete supprime les tokens stockés (logout).
func (s *TokenStore) Delete() error {
	tokenPath := filepath.Join(s.storagePath, tokenFileName)
	if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("impossible de supprimer les tokens: %w", err)
	}
	s.log.Info("tokens supprimés")
	return nil
}
