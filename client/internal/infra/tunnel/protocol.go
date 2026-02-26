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
package tunnel

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// MaxMessageSize est la taille maximale d'un message JSON autorisée
// sur le protocole tunnel (protection contre messages surdimensionnés).
const MaxMessageSize = 1 << 20 // 1 Mo

// CurrentProtocolVersion est la version courante du protocole tunnel.
const CurrentProtocolVersion = 1

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

// WriteMessage sérialise msg en JSON et l'écrit sur conn avec un
// préfixe de 4 octets (big-endian uint32) indiquant la taille du payload.
func WriteMessage(conn net.Conn, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("impossible de sérialiser le message: %w", err)
	}
	if len(data) > MaxMessageSize {
		return fmt.Errorf("message trop grand: %d octets (max %d)", len(data), MaxMessageSize)
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := conn.Write(header[:]); err != nil {
		return fmt.Errorf("impossible d'écrire le header du message: %w", err)
	}
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("impossible d'écrire le payload du message: %w", err)
	}
	return nil
}

// ReadMessage lit un message length-prefixed depuis conn et le
// désérialise dans dest.
func ReadMessage(conn net.Conn, dest any) error {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return fmt.Errorf("impossible de lire le header du message: %w", err)
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
		return fmt.Errorf("impossible de lire le payload du message: %w", err)
	}

	if err := json.Unmarshal(buf, dest); err != nil {
		return fmt.Errorf("impossible de désérialiser le message: %w", err)
	}
	return nil
}

// ParseResource convertit une chaîne de type "ssh://host:port" ou "host:port"
// en ResourceRef structuré.
func ParseResource(resource string) ResourceRef {
	// Format URI : "ssh://10.0.30.10:22"
	if strings.Contains(resource, "://") {
		u, err := url.Parse(resource)
		if err == nil && u.Hostname() != "" {
			port, _ := strconv.Atoi(u.Port())
			return ResourceRef{
				Type: u.Scheme,
				Host: u.Hostname(),
				Port: port,
			}
		}
	}

	// Format host:port : "10.0.30.10:22"
	if host, portStr, err := net.SplitHostPort(resource); err == nil {
		port, _ := strconv.Atoi(portStr)
		return ResourceRef{
			Type: "tcp",
			Host: host,
			Port: port,
		}
	}

	// Nom logique seul : "backend-ssh"
	return ResourceRef{Name: resource}
}
