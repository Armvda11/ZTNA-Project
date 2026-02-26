// Package proxy gère le relais de trafic TCP entre le client ZTNA
// (via le tunnel mTLS) et la ressource cible sur le réseau interne.
//
// Principes de sécurité fondamentaux :
//   - 1 flux = 1 ressource autorisée (pas de pivoting arbitraire)
//   - La ressource cible est fixée lors de l'autorisation et ne peut
//     pas être changée pendant la session
//   - Pas de forwarding vers des adresses non autorisées
//   - Timeouts stricts pour éviter les connexions zombies
package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"ztna-gateway/internal/config"
)

// TCPProxy relaie le trafic TCP entre le client et la ressource cible.
type TCPProxy struct {
	cfg *config.Config
	log *slog.Logger
}

// NewTCPProxy crée un nouveau proxy TCP.
func NewTCPProxy(cfg *config.Config, log *slog.Logger) *TCPProxy {
	return &TCPProxy{cfg: cfg, log: log}
}

// Proxy établit la connexion vers la ressource cible et relaie le
// trafic bidirectionnel avec la connexion client.
//
// Paramètres de sécurité :
//   - targetHost et targetPort sont FIXÉS par la décision d'autorisation
//   - Aucune redirection ou changement de cible n'est possible
//   - Le dial vers la cible utilise le timeout configuré (proxy.dial_timeout)
//
// Flux :
//  1. Établir une connexion TCP vers targetHost:targetPort
//  2. Lancer deux goroutines pour copier dans chaque direction :
//     - client → cible
//     - cible → client
//  3. Attendre la fin d'une des deux directions
//  4. Fermer proprement les deux connexions (half-close)
//  5. Journaliser les statistiques (octets transférés, durée)
//
// TODO: Implémenter le relais bidirectionnel avec :
//   - io.Copy dans deux goroutines
//   - Gestion du half-close TCP (shutdown write quand lecture terminée)
//   - Compteurs de bytes transférés (pour audit et métriques)
//   - Backpressure : si un côté est lent, l'autre est ralenti (naturel avec io.Copy)
//   - Timeout d'inactivité (idle timeout) : fermer si aucun trafic pendant N secondes
//   - Limite de durée de session (TTL du CP)
//   - Respect du contexte pour arrêt forcé (shutdown)
//   - Journalisation en fin de session : durée, bytes, erreurs
func (p *TCPProxy) Proxy(ctx context.Context, clientConn net.Conn, targetHost string, targetPort int) error {
	if err := validateTarget(targetHost, targetPort); err != nil {
		return fmt.Errorf("cible invalide: %w", err)
	}

	target := fmt.Sprintf("%s:%d", targetHost, targetPort)
	p.log.Info("ouverture de la connexion vers la ressource cible", "target", target)

	dialTimeout := p.cfg.DialTimeoutDuration()
	if dialTimeout == 0 {
		dialTimeout = 10 * time.Second
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	targetConn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return fmt.Errorf("impossible de joindre la ressource %s: %w", target, err)
	}
	defer targetConn.Close()

	p.log.Info("connexion cible établie, démarrage du relais bidirectionnel", "target", target)
	start := time.Now()

	var (
		mu           sync.Mutex
		bytesSent    int64
		bytesReceived int64
	)

	errc := make(chan error, 2)

	// Client → Cible
	go func() {
		n, err := io.Copy(targetConn, clientConn)
		mu.Lock()
		bytesSent = n
		mu.Unlock()
		// Half-close : signal EOF vers la cible
		if tc, ok := targetConn.(*net.TCPConn); ok {
			tc.CloseWrite() //nolint:errcheck
		}
		errc <- err
	}()

	// Cible → Client
	go func() {
		n, err := io.Copy(clientConn, targetConn)
		mu.Lock()
		bytesReceived = n
		mu.Unlock()
		// Half-close : signal EOF vers le client
		if tc, ok := clientConn.(*net.TCPConn); ok {
			tc.CloseWrite() //nolint:errcheck
		}
		errc <- err
	}()

	// Attendre la fin des deux directions (ou l'annulation du contexte)
	var firstErr error
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if firstErr == nil && err != nil {
				firstErr = err
			}
		case <-ctx.Done():
			targetConn.Close()
			clientConn.Close()
		}
	}

	duration := time.Since(start)
	p.log.Info("session de proxy terminée",
		"target", target,
		"duration_ms", duration.Milliseconds(),
		"bytes_sent", bytesSent,
		"bytes_received", bytesReceived,
	)

	return firstErr
}

// validateTarget vérifie que l'adresse cible est une adresse réseau
// valide et autorisée.
//
// TODO: Implémenter les vérifications :
//   - Format host:port valide
//   - Pas d'adresse loopback (127.0.0.1, ::1) sauf en lab
//   - Pas d'adresse de la Gateway elle-même (anti-loop)
//   - Optionnel : whitelist de réseaux autorisés
func validateTarget(host string, port int) error {
	if host == "" {
		return fmt.Errorf("host cible vide")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port cible invalide: %d", port)
	}
	return nil
}
