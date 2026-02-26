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

// MaxMessageSize limite la taille des messages pour protéger contre les
// clients malveillants qui enverraient des messages démesuré.
const MaxMessageSize = 1 * 1024 * 1024 // 1 Mo

// CurrentProtocolVersion est la version courante du protocole CONNECT.
const CurrentProtocolVersion = 1

// WriteMessage encode msg en JSON et l'envoie avec un préfixe de longueur
// [4 bytes big-endian uint32][JSON].
func WriteMessage(conn net.Conn, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("impossible d'encoder le message en JSON: %w", err)
	}
	if len(data) > MaxMessageSize {
		return fmt.Errorf("message trop grand: %d octets (max %d)", len(data), MaxMessageSize)
	}

	// Écrire la longueur (4 bytes big-endian)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := conn.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("impossible d'écrire la longueur: %w", err)
	}

	// Écrire le payload JSON
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("impossible d'écrire le payload: %w", err)
	}
	return nil
}

// ReadMessage lit un message length-prefixed et le décode en JSON dans dest.
func ReadMessage(conn net.Conn, dest any) error {
	// Lire les 4 bytes de longueur
	var lenBuf [4]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return fmt.Errorf("impossible de lire la longueur: %w", err)
	}
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	if msgLen == 0 {
		return fmt.Errorf("longueur du message nulle")
	}
	if int(msgLen) > MaxMessageSize {
		return fmt.Errorf("message trop grand: %d octets (max %d)", msgLen, MaxMessageSize)
	}

	// Lire le payload JSON
	payload := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return fmt.Errorf("impossible de lire le payload: %w", err)
	}

	if err := json.Unmarshal(payload, dest); err != nil {
		return fmt.Errorf("impossible de décoder le message JSON: %w", err)
	}
	return nil
}
