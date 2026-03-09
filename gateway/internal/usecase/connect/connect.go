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
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
)

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

// MaxMessageSize protège contre les messages surdimensionnés sur le tunnel.
const MaxMessageSize = 1 << 20 // 1 Mo

// WriteMessage sérialise msg en JSON et l'écrit sur conn avec un préfixe
// de 4 octets big-endian (uint32) indiquant la taille du payload.
func WriteMessage(conn net.Conn, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("sérialisation JSON: %w", err)
	}
	if len(data) > MaxMessageSize {
		return fmt.Errorf("message trop grand: %d octets (max %d)", len(data), MaxMessageSize)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := conn.Write(header[:]); err != nil {
		return fmt.Errorf("écriture header: %w", err)
	}
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("écriture payload: %w", err)
	}
	return nil
}

// ReadMessage lit un message length-prefixed depuis conn et le désérialise
// dans dest.
func ReadMessage(conn net.Conn, dest any) error {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return fmt.Errorf("lecture header: %w", err)
	}
	size := binary.BigEndian.Uint32(header[:])
	if size > MaxMessageSize {
		return fmt.Errorf("message trop grand: %d octets (max %d)", size, MaxMessageSize)
	}
	if size == 0 {
		return fmt.Errorf("message vide reçu")
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("lecture payload: %w", err)
	}
	if err := json.Unmarshal(buf, dest); err != nil {
		return fmt.Errorf("désérialisation JSON: %w", err)
	}
	return nil
}
//       // Vérifier longueur <= MaxMessageSize
//       // Lire N bytes → json.Unmarshal
//   }
