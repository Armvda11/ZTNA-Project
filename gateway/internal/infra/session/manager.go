// Package session gère le suivi des sessions actives sur la Gateway.
// Une session représente une connexion mTLS client autorisée qui est
// en cours de proxy vers une ressource cible.
//
// Le gestionnaire de sessions permet :
//   - Le suivi des connexions actives par sujet (sub)
//   - L'application de limites de connexions par sujet
//   - Les timeouts d'inactivité et de durée maximale
//   - L'exposition de métriques (nombre de sessions, durées, etc.)
package session

import (
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

// Register enregistre une nouvelle session active.
//
// TODO: Implémenter :
//   - Vérification des limites de connexions par sujet (max_conns_per_subject)
//   - Générer un ID unique pour la session (UUID)
//   - Journaliser l'ouverture de session
//   - Démarrer un timer pour le timeout d'inactivité
//   - Démarrer un timer pour la durée maximale (TTL du CP)
func (m *Manager) Register(s *Session) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// TODO: vérifier la limite de connexions par sujet
	//   count := 0
	//   for _, sess := range m.sessions {
	//       if sess.Sub == s.Sub { count++ }
	//   }
	//   if count >= maxConnsPerSubject { return "", ErrTooManySessions }

	// TODO: générer un ID unique (UUID)
	// sessionID := uuid.New().String()
	sessionID := "TODO-session-id"

	s.ID = sessionID
	s.StartedAt = time.Now()
	m.sessions[sessionID] = s

	m.log.Info("session enregistrée",
		"session_id", sessionID,
		"sub", s.Sub,
		"resource", s.ResourceHost,
		"port", s.ResourcePort,
	)

	return sessionID, nil
}

// Unregister supprime une session terminée.
//
// TODO: Journaliser la fin de session avec les métriques :
//   - Durée de la session
//   - Bytes transférés (client→cible, cible→client)
//   - Raison de fermeture (fin normale, timeout, erreur)
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
		"resource", s.ResourceHost,
		"duration", duration.String(),
	)

	delete(m.sessions, sessionID)
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

// TODO: Ajouter des métriques exposables :
//   - Nombre total de sessions actives (gauge)
//   - Nombre de sessions par sujet (gauge)
//   - Durée moyenne des sessions (histogram)
//   - Sessions ouvertes/fermées (counter)
//   - Sessions refusées pour limite atteinte (counter)
//
// TODO: Ajouter un garbage collector pour les sessions zombies
//   - Vérifier périodiquement les sessions inactives
//   - Fermer les sessions qui dépassent le TTL
