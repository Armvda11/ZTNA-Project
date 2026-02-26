// Package tunnel gère l'établissement du tunnel mTLS entre le client ZTNA
// et la Gateway. Il configure la connexion TLS avec le certificat client
// obtenu via le Control Plane et gère le handshake CONNECT.
package tunnel

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"client/internal/config"
	"client/internal/core/domain"
)

// Manager gère les connexions tunnel mTLS vers la Gateway.
type Manager struct {
	cfg *config.Config
	log *slog.Logger
}

// NewManager crée un nouveau gestionnaire de tunnel.
func NewManager(cfg *config.Config, log *slog.Logger) *Manager {
	return &Manager{cfg: cfg, log: log}
}

// buildTLSConfig construit la configuration TLS pour la connexion
// à la Gateway.
//
// Paramètres de sécurité :
//   - TLS 1.3 minimum (pas de négociation vers des versions antérieures)
//   - Certificat client mTLS chargé depuis le stockage
//   - RootCAs chargé depuis le fichier CA configuré (tls.ca_file)
//   - Vérification du ServerName activée (pas de skip)
//
// TODO: Ajouter le support des cipher suites spécifiques si nécessaire
// TODO: Ajouter le certificate pinning optionnel pour la Gateway
func (m *Manager) buildTLSConfig(certPEM, keyPEM []byte) (*tls.Config, error) {
	// Charger le certificat client
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("impossible de charger le certificat client: %w", err)
	}

	// Charger la CA de confiance pour vérifier le certificat de la Gateway
	var rootCAs *x509.CertPool
	if m.cfg.TLS.CAFile != "" {
		caCert, err := os.ReadFile(m.cfg.TLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("impossible de lire le fichier CA %s: %w", m.cfg.TLS.CAFile, err)
		}
		rootCAs = x509.NewCertPool()
		if !rootCAs.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("impossible de parser le certificat CA depuis %s", m.cfg.TLS.CAFile)
		}
	}

	// Extraire le hostname depuis l'adresse de la Gateway (host:port)
	serverName, _, err := net.SplitHostPort(m.cfg.Gateway.Address)
	if err != nil {
		serverName = m.cfg.Gateway.Address
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		RootCAs:      rootCAs,
		ServerName:   serverName,
	}

	return tlsConfig, nil
}

// Connect établit une connexion TLS vers la Gateway et effectue le
// handshake CONNECT pour accéder à la ressource demandée.
//
// Flux :
//  1. Charger le certificat mTLS client
//  2. Construire la tls.Config
//  3. Établir la connexion TLS vers gateway.address
//  4. Envoyer une requête CONNECT (voir protocol.go)
//  5. Lire la réponse de la Gateway
//  6. Si allow : retourner la connexion pour le relais de trafic
//  7. Si deny : fermer la connexion et retourner l'erreur
//
// TODO: Supporter la reconnexion automatique en cas de perte de connexion
func (m *Manager) Connect(certPEM, keyPEM []byte, resource string) (net.Conn, error) {
	m.log.Info("établissement du tunnel mTLS", "gateway", m.cfg.Gateway.Address, "resource", resource)

	tlsConfig, err := m.buildTLSConfig(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("erreur de configuration TLS: %w", err)
	}

	// Établir la connexion TLS avec timeout
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", m.cfg.Gateway.Address, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: impossible de se connecter à la Gateway %s: %v",
			domain.ErrGatewayUnreachable, m.cfg.Gateway.Address, err)
	}

	// Construire et envoyer la ConnectRequest
	resRef := ParseResource(resource)
	req := ConnectRequest{
		ProtocolVersion: CurrentProtocolVersion,
		Action:          "connect",
		Resource:        resRef,
		Context: ConnectContext{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := WriteMessage(conn, req); err != nil {
		conn.Close()
		return nil, fmt.Errorf("impossible d'envoyer la requête CONNECT: %w", err)
	}

	// Lire la réponse de la Gateway
	var resp ConnectResponse
	if err := ReadMessage(conn, &resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("impossible de lire la réponse CONNECT: %w", err)
	}

	if resp.Decision != "allow" {
		conn.Close()
		reason := resp.Reason
		if reason == "" {
			reason = "raison non communiquée"
		}
		return nil, fmt.Errorf("%w: %s (decision_id=%s)", domain.ErrConnectionDenied, reason, resp.DecisionID)
	}

	m.log.Info("tunnel mTLS établi", "decision_id", resp.DecisionID, "ttl_seconds", resp.TTLSeconds)
	return conn, nil
}

// RelayTraffic relaie le trafic bidirectionnel entre la connexion locale
// et le tunnel mTLS.
//
// TODO: Ajouter les compteurs de trafic pour audit
// TODO: Arrêt propre via context cancellation
func (m *Manager) RelayTraffic(tunnel net.Conn, local net.Conn) error {
	m.log.Debug("démarrage du relais de trafic bidirectionnel")

	errc := make(chan error, 2)

	// Tunnel → Local
	go func() {
		_, err := io.Copy(local, tunnel)
		errc <- err
	}()

	// Local → Tunnel
	go func() {
		_, err := io.Copy(tunnel, local)
		errc <- err
	}()

	// Attendre la première erreur (une direction se ferme)
	err := <-errc

	// Fermer les deux côtés pour débloquer la goroutine restante
	tunnel.Close()
	local.Close()

	// Attendre la seconde goroutine
	<-errc

	if err != nil {
		m.log.Debug("relais de trafic terminé", "error", err)
	} else {
		m.log.Debug("relais de trafic terminé proprement")
	}

	return err
}
