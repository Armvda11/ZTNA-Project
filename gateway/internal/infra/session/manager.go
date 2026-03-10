// Package session gère le suivi des sessions actives sur la Gateway.
// Une session représente une connexion mTLS client autorisée qui est
// en cours de proxy vers une ressource cible.
//
// Le gestionnaire de sessions permet :
//   - Le suivi des connexions actives par sujet (sub)
//   - L'application de limites de connexions par sujet
//   - Les timeouts de durée maximale basés sur le TTL du CP
//   - La fermeture forcée de sessions (admin kill)
//   - L'exposition de métriques (nombre de sessions, durées, etc.)
//   - Le garbage collection des sessions zombies
package session

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Session représente une connexion active sur la Gateway.
type Session struct {
	// ID unique de la session
	ID string

	// Sub est l'identifiant OIDC de l'utilisateur
	Sub string

	// Username est le nom d'utilisateur
	Username string

	// ResourceType est le type de ressource accédée
	ResourceType string

	// ResourceName est le nom de la ressource publiée (si résolu via CP)
	ResourceName string

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

	// TTLSeconds est la durée de vie accordée par le CP (0 = illimité)
	TTLSeconds int

	// ExpiresAt est calculé à partir de StartedAt + TTLSeconds
	ExpiresAt time.Time

	// CancelFunc permet de fermer la session de force (admin kill / TTL)
	CancelFunc context.CancelFunc

	// BytesIn compteur de bytes client→cible
	BytesIn int64

	// BytesOut compteur de bytes cible→client
	BytesOut int64

	// EndReason raison de fin de session
	EndReason string

	// CertSerial est le numéro de série du certificat client (hex)
	CertSerial string
}

// Manager gère les sessions actives sur la Gateway.
type Manager struct {
	log            *slog.Logger
	mu             sync.RWMutex
	sessions       map[string]*Session
	maxPerSubject  int           // 0 = illimité
	gcInterval     time.Duration // intervalle du garbage collector
}

// NewManager crée un nouveau gestionnaire de sessions.
func NewManager(log *slog.Logger) *Manager {
	return &Manager{
		log:           log,
		sessions:      make(map[string]*Session),
		maxPerSubject: 10,            // limite raisonnable par défaut
		gcInterval:    30 * time.Second,
	}
}

// NewManagerWithLimits crée un gestionnaire avec des limites configurables.
func NewManagerWithLimits(log *slog.Logger, maxPerSubject int) *Manager {
	m := NewManager(log)
	if maxPerSubject > 0 {
		m.maxPerSubject = maxPerSubject
	}
	return m
}

// Register enregistre une nouvelle session active.
// Vérifie la limite de connexions par sujet avant d'enregistrer.
// CancelFunc peut être nil si pas de support de kill.
func (m *Manager) Register(s *Session) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Vérifier la limite par sujet
	if m.maxPerSubject > 0 {
		count := 0
		for _, existing := range m.sessions {
			if existing.Sub == s.Sub {
				count++
			}
		}
		if count >= m.maxPerSubject {
			m.log.Warn("limite de sessions atteinte",
				"sub", s.Sub,
				"current", count,
				"max", m.maxPerSubject,
			)
			return "", fmt.Errorf("limite de sessions atteinte (%d/%d) pour %s", count, m.maxPerSubject, s.Sub)
		}
	}

	// Générer un ID unique (UUID v4)
	sessionID := generateUUID()

	s.ID = sessionID
	s.StartedAt = time.Now()

	// Calculer l'expiration si TTL > 0
	if s.TTLSeconds > 0 {
		s.ExpiresAt = s.StartedAt.Add(time.Duration(s.TTLSeconds) * time.Second)
	}

	m.sessions[sessionID] = s

	m.log.Info("session enregistrée",
		"session_id", sessionID,
		"sub", s.Sub,
		"resource", fmt.Sprintf("%s://%s:%d", s.ResourceType, s.ResourceHost, s.ResourcePort),
		"ttl_seconds", s.TTLSeconds,
		"expires_at", s.ExpiresAt.Format(time.RFC3339),
		"active_count", len(m.sessions),
	)

	return sessionID, nil
}

// Unregister supprime une session terminée et journalise les métriques.
func (m *Manager) Unregister(sessionID string) {
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	duration := time.Since(s.StartedAt)
	endReason := s.EndReason
	if endReason == "" {
		endReason = "normal"
	}

	m.log.Info("session terminée",
		"session_id", sessionID,
		"sub", s.Sub,
		"resource", fmt.Sprintf("%s://%s:%d", s.ResourceType, s.ResourceHost, s.ResourcePort),
		"duration_ms", duration.Milliseconds(),
		"bytes_in", s.BytesIn,
		"bytes_out", s.BytesOut,
		"end_reason", endReason,
	)
}

// KillSession force la fermeture d'une session active (admin/TTL).
// La session est immédiatement retirée de la map et son CancelFunc appelé.
func (m *Manager) KillSession(sessionID string) error {
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %s non trouvée", sessionID)
	}
	s.EndReason = "admin_kill"
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	if s.CancelFunc != nil {
		s.CancelFunc()
	}

	m.log.Warn("session tuée par admin",
		"session_id", sessionID,
		"sub", s.Sub,
	)

	return nil
}

// KillBySerial force la fermeture de toutes les sessions liées à un serial de certificat.
// Retourne le nombre de sessions tuées.
func (m *Manager) KillBySerial(serial string) int {
	if serial == "" {
		return 0
	}

	m.mu.Lock()
	var toKill []*Session
	for id, s := range m.sessions {
		if s.CertSerial == serial {
			s.EndReason = "cert_revoked"
			toKill = append(toKill, s)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()

	for _, s := range toKill {
		if s.CancelFunc != nil {
			s.CancelFunc()
		}
		m.log.Warn("session tuée — certificat révoqué",
			"session_id", s.ID,
			"sub", s.Sub,
			"cert_serial", s.CertSerial,
		)
	}

	return len(toKill)
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

// ListActive retourne une copie de toutes les sessions actives.
func (m *Manager) ListActive() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	return list
}

// GetSession retourne une session par ID.
func (m *Manager) GetSession(sessionID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[sessionID]
	return s, ok
}

// SetEndStats met à jour les statistiques de fin d'une session.
func (m *Manager) SetEndStats(sessionID string, bytesIn, bytesOut int64, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		s.BytesIn = bytesIn
		s.BytesOut = bytesOut
		if reason != "" {
			s.EndReason = reason
		}
	}
}

// StartGarbageCollector lance un goroutine qui vérifie périodiquement
// les sessions expirées (TTL dépassé) et les termine.
func (m *Manager) StartGarbageCollector(ctx context.Context) {
	ticker := time.NewTicker(m.gcInterval)
	defer ticker.Stop()

	m.log.Info("garbage collector de sessions démarré", "interval", m.gcInterval.String())

	for {
		select {
		case <-ctx.Done():
			m.log.Info("garbage collector de sessions arrêté")
			return
		case <-ticker.C:
			m.reapExpired()
		}
	}
}

// reapExpired tue les sessions dont le TTL a expiré.
func (m *Manager) reapExpired() {
	now := time.Now()
	m.mu.RLock()
	var expired []string
	for id, s := range m.sessions {
		if s.TTLSeconds > 0 && now.After(s.ExpiresAt) {
			expired = append(expired, id)
		}
	}
	m.mu.RUnlock()

	for _, id := range expired {
		m.mu.Lock()
		s, ok := m.sessions[id]
		if ok {
			s.EndReason = "ttl_expired"
			delete(m.sessions, id)
		}
		m.mu.Unlock()

		if ok && s.CancelFunc != nil {
			m.log.Warn("session expirée — fermeture forcée",
				"session_id", id,
				"sub", s.Sub,
				"ttl_seconds", s.TTLSeconds,
				"elapsed", time.Since(s.StartedAt).String(),
			)
			s.CancelFunc()
		}
	}

	if len(expired) > 0 {
		m.log.Info("garbage collector: sessions expirées nettoyées", "count", len(expired))
	}
}

// generateUUID produit un UUID v4 sans dépendance externe.
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
