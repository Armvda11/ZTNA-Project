// Package ports définit les interfaces cœur pour découpler use-cases et implémentations infra.
//
// Chaque interface correspond à une capacité de la Gateway ZTNA.
// Les implémentations concrètes résident dans internal/infra/.
// Les use-cases ne dépendent que de ces interfaces (inversion de dépendance).
package ports

import (
	"context"
	"crypto/x509"
	"net"

	"ztna-gateway/internal/core/domain"
)

// HealthChecker — diagnostic de santé d'un composant.
type HealthChecker interface {
	Health(ctx context.Context) error
}

// Authorizer évalue une requête d'accès auprès du Control Plane.
type Authorizer interface {
	// Authorize envoie la requête au CP et retourne la décision.
	Authorize(req *AuthzRequest) (*AuthzResponse, error)

	// GatewayID retourne l'identifiant PEP de cette gateway.
	GatewayID() string
}

// AuthzRequest est la requête d'autorisation pour le CP.
type AuthzRequest struct {
	Subject  domain.SubjectRef
	Action   string
	Resource domain.ResourceRef
	Context  AuthzContext
}

// AuthzContext contient le contexte réseau de la requête.
type AuthzContext struct {
	SourceIP  string
	GatewayID string
}

// AuthzResponse est la décision retournée par le CP.
type AuthzResponse struct {
	Decision      string
	TTLSeconds    int
	Reason        string
	PolicyVersion int64
	DecisionID    string
}

// Proxy relaie le trafic entre le client mTLS et la ressource cible.
type Proxy interface {
	// Proxy établit la connexion cible et relaie bidirectionnellement.
	Proxy(ctx context.Context, clientConn net.Conn, targetHost string, targetPort int) error
}

// SessionManager gère le cycle de vie des sessions actives.
type SessionManager interface {
	// Register enregistre une nouvelle session. Retourne l'ID de session.
	Register(s *SessionInfo) (string, error)

	// Unregister termine une session.
	Unregister(sessionID string)

	// ActiveCount retourne le nombre total de sessions actives.
	ActiveCount() int

	// ActiveCountForSubject retourne le nombre de sessions d'un sujet.
	ActiveCountForSubject(sub string) int

	// KillSession force la fermeture d'une session active.
	KillSession(sessionID string) error

	// ListActive retourne toutes les sessions actives.
	ListActive() []*SessionInfo
}

// SessionInfo décrit une session active (vue port).
type SessionInfo struct {
	ID           string
	Sub          string
	Username     string
	ResourceType string
	ResourceHost string
	ResourcePort int
	SourceIP     string
	DecisionID   string
	TTLSeconds   int
}

// RevocationChecker vérifie si un certificat a été révoqué.
type RevocationChecker interface {
	// IsRevoked retourne true si le serial number est dans la CRL.
	IsRevoked(serial string) bool

	// StartAutoRefresh lance le téléchargement périodique de la CRL.
	StartAutoRefresh(ctx context.Context) error
}

// DecisionCache met en cache les décisions d'autorisation du CP.
type DecisionCache interface {
	Get(key string) (*AuthzResponse, bool)
	Put(key string, resp *AuthzResponse, ttlSeconds int)
	Clear()
}

// SessionTelemetry envoie la télémétrie de session au CP.
type SessionTelemetry interface {
	// NotifySessionStart informe le CP qu'une session a démarré.
	NotifySessionStart(ctx context.Context, info SessionTelemetryStart) error

	// NotifySessionEnd informe le CP qu'une session s'est terminée.
	NotifySessionEnd(ctx context.Context, info SessionTelemetryEnd) error
}

// SessionTelemetryStart données envoyées au CP au début d'une session.
type SessionTelemetryStart struct {
	SessionID      string `json:"session_id"`
	DecisionID     string `json:"decision_id"`
	SubjectSub     string `json:"subject_sub"`
	SubjectUser    string `json:"subject_username"`
	ResourceType   string `json:"resource_type"`
	ResourceMatch  string `json:"resource_match"`
	DeviceSerial   string `json:"device_serial,omitempty"`
}

// SessionTelemetryEnd données envoyées au CP en fin de session.
type SessionTelemetryEnd struct {
	SessionID  string `json:"session_id"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
	DurationMs int64  `json:"duration_ms"`
	EndReason  string `json:"end_reason"`
}

// ConnectionHandler traite une connexion entrante (implémenté par le handler CONNECT).
type ConnectionHandler interface {
	HandleConnection(conn net.Conn, clientCert *x509.Certificate)
}
