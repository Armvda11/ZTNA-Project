// Package tunnel gère l'établissement du tunnel mTLS entre le client ZTNA
// et la Gateway. Il configure la connexion TLS avec le certificat client
// obtenu via le Control Plane et gère le handshake CONNECT.
package tunnel

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"os"

	"client/internal/config"
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
// TODO: Implémenter le handshake CONNECT complet
// TODO: Implémenter le relais de trafic bidirectionnel
// TODO: Gérer les timeouts de connexion et de handshake
// TODO: Supporter la reconnexion automatique en cas de perte de connexion
func (m *Manager) Connect(certPEM, keyPEM []byte, resource string) (net.Conn, error) {
	m.log.Info("établissement du tunnel mTLS", "gateway", m.cfg.Gateway.Address, "resource", resource)

	tlsConfig, err := m.buildTLSConfig(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("erreur de configuration TLS: %w", err)
	}

	// TODO: établir la connexion TLS
	//   conn, err := tls.Dial("tcp", m.cfg.Gateway.Address, tlsConfig)
	_ = tlsConfig

	// TODO: construire et envoyer la ConnectRequest (voir protocol.go)
	//   req := protocol.ConnectRequest{
	//       Action:   "connect",
	//       Resource: protocol.ResourceRef{...},
	//       Context:  protocol.ConnectContext{...},
	//   }

	// TODO: lire la ConnectResponse
	// TODO: vérifier Decision == "allow"
	// TODO: retourner la connexion pour le relais

	return nil, fmt.Errorf("TODO: Connect non implémenté")
}

// RelayTraffic relaie le trafic bidirectionnel entre la connexion locale
// et le tunnel mTLS.
//
// TODO: Implémenter le relais avec :
//   - io.Copy bidirectionnel (goroutines)
//   - Gestion du half-close TCP
//   - Timeouts d'inactivité
//   - Compteurs de trafic pour audit
//   - Arrêt propre via context cancellation
func (m *Manager) RelayTraffic(tunnel net.Conn, local net.Conn) error {
	m.log.Debug("démarrage du relais de trafic bidirectionnel")

	// TODO: lancer deux goroutines pour copier dans chaque direction
	// TODO: gérer la fermeture propre quand une direction se termine
	// TODO: respecter le contexte pour l'arrêt forcé

	return fmt.Errorf("TODO: RelayTraffic non implémenté")
}
