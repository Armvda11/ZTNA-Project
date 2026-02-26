// Package session gère le suivi des sessions actives sur la Gateway.
// Une session représente une connexion mTLS client autorisée qui est
// en cours de proxy vers une ressource cible.
//
// Le gestionnaire de sessions permet :
//   - Le suivi des connexions actives par sujet (sub)
//   - L'application de limites de connexions par sujet (max 10)
//   - La terminaison immédiate d'une session via KillSession
//   - La terminaison de toutes les sessions pour les certs révoqués (KillRevoked)
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const maxConnsPerSubject = 10

// ErrTooManySessions est retourné quand un sujet dépasse la limite de sessions actives.
var ErrTooManySessions = errors.New("limite de sessions actives atteinte pour ce sujet")

// Session représente une connexion active sur la Gateway.
type Session struct {
	// ID unique de la session (UUID hex 32 chars)
	ID string

	// Sub est l'identifiant OIDC de l'utilisateur
	Sub string

	// Username est le nom d'utilisateur
	Username string

	// ResourceType est le type de ressource accédée
	ResourceType string

	// ResourceHost est l'adresse de la ressource cible
	ResourceHost string

	// ResourcePort est le port de la ressource cible
	ResourcePort int

	// StartedAt est l'horodatage de début de session
	StartedAt time.Time

	// SourceIP est l'adresse IP du client
	SourceIP string

	// DecisionID est l'identifiant de la décision d'autorisation du CP
	DecisionID string

	// DeviceSerial est le serial du certificat device utilisé
	DeviceSerial string

	// cancel ferme le contexte du proxy associé à cette session
	cancel context.CancelFunc
}

// Manager gère les sessions actives sur la Gateway.
type Manager struct {
	log      *slog.Logger
	mu       sync.RWMutex
	sessions map[string]*Session // sessionID → Session
}

// NewManager crée un nouveau gestionnaire de sessions.
func NewManager(log *slog.Logger) *Manager {
	return &Manager{
		log:      log,
		sessions: make(map[string]*Session),
	}
}

// newSessionID génère un identifiant de session unique (UUID v4 simplifié : 16 bytes hex).
func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("génération UUID session impossible: %w", err)
	}
	// Format UUID v4 : xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]), nil
}

// Register enregistre une nouvelle session active.
// cancel est la fonction d'annulation du contexte associé au proxy ;
// elle sera appelée par KillSession ou KillRevoked pour couper la session en live.
func (m *Manager) Register(s *Session, cancel context.CancelFunc) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Vérifier la limite de connexions par sujet
	count := 0
	for _, sess := range m.sessions {
		if sess.Sub == s.Sub {
			count++
		}
	}
	if count >= maxConnsPerSubject {
		return "", fmt.Errorf("%w (sujet=%s, actuel=%d)", ErrTooManySessions, s.Sub, count)
	}

	sessionID, err := newSessionID()
	if err != nil {
		return "", err
	}

	s.ID = sessionID
	s.StartedAt = time.Now()
	s.cancel = cancel
	m.sessions[sessionID] = s

	m.log.Info("session enregistrée",
		"session_id", sessionID,
		"sub", s.Sub,
		"resource", fmt.Sprintf("%s:%d", s.ResourceHost, s.ResourcePort),
		"device_serial", s.DeviceSerial,
	)

	return sessionID, nil
}

// Unregister supprime une session terminée et libère ses ressources.
func (m *Manager) Unregister(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return
	}

	duration := time.Since(s.StartedAt)
	m.log.Info("session terminée",
		"session_id", sessionID,
		"sub", s.Sub,
		"resource", fmt.Sprintf("%s:%d", s.ResourceHost, s.ResourcePort),
		"duration", duration.String(),
	)

	delete(m.sessions, sessionID)
}

// KillSession termine immédiatement une session active par son ID.
// Cela appelle cancel() sur le contexte du proxy, ce qui ferme le tunnel TCP.
// Retourne false si la session n'existe pas.
func (m *Manager) KillSession(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return false
	}

	m.log.Info("kill session (admin)",
		"session_id", sessionID,
		"sub", s.Sub,
		"resource", fmt.Sprintf("%s:%d", s.ResourceHost, s.ResourcePort),
	)

	if s.cancel != nil {
		s.cancel()
	}
	return true
}

// KillRevoked termine toutes les sessions actives dont le serial de certificat
// figure dans la liste revokedSerials. Appelé après chaque refresh de CRL.
func (m *Manager) KillRevoked(revokedSerials []string) {
	if len(revokedSerials) == 0 {
		return
	}

	revoked := make(map[string]struct{}, len(revokedSerials))
	for _, s := range revokedSerials {
		revoked[s] = struct{}{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, sess := range m.sessions {
		if _, isRevoked := revoked[sess.DeviceSerial]; isRevoked {
			m.log.Info("kill session (cert révoqué)",
				"session_id", id,
				"sub", sess.Sub,
				"device_serial", sess.DeviceSerial,
			)
			if sess.cancel != nil {
				sess.cancel()
			}
		}
	}
}

// ActiveCount retourne le nombre de sessions actives.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// ActiveCountForSubject retourne le nombre de sessions actives pour un sujet.
func (m *Manager) ActiveCountForSubject(sub string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, s := range m.sessions {
		if s.Sub == sub {
			count++
		}
	}
	return count
}
//   - Sessions refusées pour limite atteinte (counter)
//
// TODO: Ajouter un garbage collector pour les sessions zombies
//   - Vérifier périodiquement les sessions inactives
//   - Fermer les sessions qui dépassent le TTL
