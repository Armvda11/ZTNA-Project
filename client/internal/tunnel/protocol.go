// Package tunnel — protocol.go
//
// Définit les structures de messages échangés entre le client et la
// Gateway lors du handshake CONNECT. Ce protocole s'exécute par-dessus
// le tunnel mTLS déjà établi.
//
// Design actuel : JSON length-prefixed sur le flux TLS.
// Évolutions futures possibles :
//   - HTTP/2 multiplexé (plusieurs flux par connexion)
//   - gRPC bidirectionnel
//   - Protocol Buffers pour la sérialisation
//
// Le format de trame est :
//
//	[4 bytes : longueur du message JSON en big-endian uint32]
//	[N bytes : message JSON]
//
// TODO: Finaliser le choix du protocole de framing
// TODO: Ajouter un numéro de version du protocole dans le handshake
package tunnel

// ConnectRequest est la requête envoyée par le client à la Gateway
// pour demander l'accès à une ressource.
type ConnectRequest struct {
	// Version du protocole (pour compatibilité future)
	ProtocolVersion int `json:"protocol_version"`

	// Action demandée (actuellement toujours "connect")
	Action string `json:"action"`

	// Resource ciblée
	Resource ResourceRef `json:"resource"`

	// Context additionnel pour l'évaluation de la politique
	Context ConnectContext `json:"context"`
}

// ResourceRef identifie une ressource réseau cible.
type ResourceRef struct {
	// Type de ressource : "ssh", "tcp", "http", "rdp", etc.
	Type string `json:"type"`

	// Host cible (IP ou FQDN)
	Host string `json:"host"`

	// Port cible
	Port int `json:"port"`

	// Name ou identifiant optionnel de la ressource (pour les politiques)
	Name string `json:"name,omitempty"`
}

// ConnectContext contient les informations contextuelles envoyées avec
// la requête CONNECT pour enrichir l'évaluation de politique.
type ConnectContext struct {
	// SourceIP du client (peut être rempli côté Gateway si absent)
	SourceIP string `json:"src_ip,omitempty"`

	// DeviceInfo contient des informations sur le poste client (optionnel)
	// TODO: Définir le schéma des informations device (OS, version, posture)
	DeviceInfo map[string]string `json:"device_info,omitempty"`

	// Timestamp de la requête (ISO 8601)
	Timestamp string `json:"timestamp,omitempty"`
}

// ConnectResponse est la réponse de la Gateway à une requête CONNECT.
type ConnectResponse struct {
	// Decision : "allow" ou "deny"
	Decision string `json:"decision"`

	// Reason explique le motif de la décision (surtout utile pour deny)
	Reason string `json:"reason,omitempty"`

	// DecisionID identifiant unique de la décision (pour audit/traçabilité)
	DecisionID string `json:"decision_id,omitempty"`

	// TTLSeconds durée de validité de la session autorisée
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

// TODO: Implémenter les fonctions de sérialisation/désérialisation
//       avec framing length-prefixed :
//
//   func WriteMessage(conn net.Conn, msg any) error
//   func ReadMessage(conn net.Conn, dest any) error
//
// TODO: Ajouter une constante pour la taille maximale de message
//       (protection contre les attaques par messages surdimensionnés)
//
// TODO: Ajouter un timeout de lecture pour éviter les blocages
