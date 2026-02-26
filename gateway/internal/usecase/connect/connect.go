// Package protocol — connect.go
//
// Définit les structures de données pour le protocole CONNECT échangé
// entre le client et la Gateway. Le client envoie une ConnectRequest
// pour demander l'accès à une ressource ; la Gateway répond avec une
// ConnectResponse après consultation du Control Plane.
//
// Framing actuel : JSON length-prefixed sur le flux TLS.
//
//	[4 bytes : longueur du message JSON en big-endian uint32]
//	[N bytes : message JSON]
//
// Évolutions futures :
//   - HTTP/2 pour multiplexer plusieurs flux
//   - gRPC pour typage fort et streaming
//   - Protocol Buffers pour la sérialisation
//
// TODO: Finaliser le choix du protocole de framing
// TODO: Ajouter un numéro de version du protocole
package protocol

// ConnectRequest est la requête envoyée par le client à la Gateway.
type ConnectRequest struct {
	// ProtocolVersion permet la négociation de version future
	ProtocolVersion int `json:"protocol_version"`

	// Action demandée (actuellement "connect")
	Action string `json:"action"`

	// Resource ciblée par la connexion
	Resource ResourceTarget `json:"resource"`

	// Context additionnel pour l'évaluation des politiques
	Context RequestContext `json:"context"`
}

// ResourceTarget identifie la ressource réseau demandée.
type ResourceTarget struct {
	Type string `json:"type"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Name string `json:"name,omitempty"`
}

// RequestContext contient les informations contextuelles de la requête.
type RequestContext struct {
	SourceIP   string            `json:"src_ip,omitempty"`
	DeviceInfo map[string]string `json:"device_info,omitempty"`
	Timestamp  string            `json:"timestamp,omitempty"`
}

// ConnectResponse est la réponse de la Gateway au client.
type ConnectResponse struct {
	// Decision : "allow" ou "deny"
	Decision string `json:"decision"`

	// Reason du refus (si Decision == "deny")
	Reason string `json:"reason,omitempty"`

	// DecisionID pour traçabilité (retourné par le CP)
	DecisionID string `json:"decision_id,omitempty"`

	// TTLSeconds durée maximale de la session autorisée
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

// TODO: Implémenter les fonctions de framing :
//
//   const MaxMessageSize = 64 * 1024 // 64 KB max par message
//
//   // WriteMessage encode un message en JSON et l'envoie avec le préfixe de longueur.
//   func WriteMessage(conn net.Conn, msg any) error {
//       data, err := json.Marshal(msg)
//       // Vérifier len(data) <= MaxMessageSize
//       // Écrire [4 bytes big-endian len][data]
//   }
//
//   // ReadMessage lit un message length-prefixed et le décode en JSON.
//   func ReadMessage(conn net.Conn, dest any) error {
//       // Lire 4 bytes → longueur
//       // Vérifier longueur <= MaxMessageSize
//       // Lire N bytes → json.Unmarshal
//   }
