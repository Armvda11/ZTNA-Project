// Package domain définit les modèles de données partagés par le client ZTNA.
// Ces types sont utilisés par les différents packages internes pour
// représenter les concepts métier de manière cohérente.
package domain

// ResourceRef identifie une ressource réseau accessible via le ZTNA.
type ResourceRef struct {
	// Type de la ressource : "ssh", "tcp", "http", "rdp", etc.
	Type string `json:"type"`

	// Host est l'adresse IP ou le FQDN de la ressource cible.
	Host string `json:"host"`

	// Port est le port réseau de la ressource cible.
	Port int `json:"port"`

	// Name est un identifiant humain optionnel (ex: "serveur-web-prod").
	Name string `json:"name,omitempty"`
}

// SubjectRef identifie un utilisateur authentifié dans le système ZTNA.
type SubjectRef struct {
	// Sub est l'identifiant unique OIDC du sujet (claim "sub").
	Sub string `json:"sub"`

	// Username est le nom d'utilisateur lisible (claim "preferred_username").
	Username string `json:"username"`

	// Groups contient la liste des groupes auxquels l'utilisateur appartient.
	//
	// ⚠️  ATTENTION STALENESS : Les groupes proviennent du token OIDC et
	// peuvent être obsolètes si l'utilisateur a été ajouté/retiré d'un
	// groupe depuis la dernière authentification. Pour les décisions
	// critiques, le Control Plane devrait re-vérifier les groupes.
	Groups []string `json:"groups,omitempty"`
}

// ConnectRequest représente une demande de connexion à une ressource.
type ConnectRequest struct {
	Action   string      `json:"action"`
	Resource ResourceRef `json:"resource"`
	Context  RequestContext `json:"context"`
}

// ConnectResponse représente la réponse à une demande de connexion.
type ConnectResponse struct {
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	DecisionID string `json:"decision_id,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

// RequestContext contient les informations contextuelles d'une requête.
type RequestContext struct {
	SourceIP   string            `json:"src_ip,omitempty"`
	DeviceInfo map[string]string `json:"device_info,omitempty"`
	Timestamp  string            `json:"timestamp,omitempty"`
}
