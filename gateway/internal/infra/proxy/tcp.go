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
	"sync/atomic"
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
func (p *TCPProxy) Proxy(ctx context.Context, clientConn net.Conn, targetHost string, targetPort int) error {
	if err := validateTarget(targetHost, targetPort); err != nil {
		return fmt.Errorf("cible invalide: %w", err)
	}

	target := fmt.Sprintf("%s:%d", targetHost, targetPort)
	p.log.Info("ouverture connexion vers la ressource cible", "target", target)

	// Établir la connexion vers la cible avec timeout
	dialTimeout := p.cfg.DialTimeoutDuration()
	dialer := &net.Dialer{Timeout: dialTimeout}
	targetConn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return fmt.Errorf("impossible de joindre %s: %w", target, err)
	}
	defer targetConn.Close()

	p.log.Info("connexion établie vers la ressource",
		"target", target,
		"local_addr", targetConn.LocalAddr().String(),
	)

	startTime := time.Now()
	var bytesClientToTarget, bytesTargetToClient atomic.Int64

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	// client → cible
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := io.Copy(targetConn, clientConn)
		bytesClientToTarget.Store(n)
		// Half-close: signaler à la cible qu'on n'écrira plus
		if tc, ok := targetConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		errCh <- err
	}()

	// cible → client
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := io.Copy(clientConn, targetConn)
		bytesTargetToClient.Store(n)
		// Half-close: signaler au client qu'on n'écrira plus
		if tc, ok := clientConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		errCh <- err
	}()

	// Attendre qu'un des deux côtés se termine
	firstErr := <-errCh

	// Forcer la fermeture pour débloquer l'autre goroutine
	clientConn.Close()
	targetConn.Close()

	wg.Wait()
	duration := time.Since(startTime)

	p.log.Info("session proxy terminée",
		"target", target,
		"duration", duration.Round(time.Millisecond).String(),
		"bytes_client_to_target", bytesClientToTarget.Load(),
		"bytes_target_to_client", bytesTargetToClient.Load(),
	)

	return firstErr
}

// validateTarget vérifie que l'adresse cible est une adresse réseau
// valide et autorisée. Protège contre les attaques SSRF.
//
// Bloque:
//   - Adresses loopback (127.0.0.0/8, ::1)
//   - Adresses link-local (169.254.0.0/16, fe80::/10)
//   - Cloud metadata endpoints (169.254.169.254)
//   - Adresses multicast et broadcast
//   - Ports invalides (<1 ou >65535)
func validateTarget(host string, port int) error {
	if host == "" {
		return fmt.Errorf("host cible vide")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port cible invalide: %d", port)
	}

	// Résoudre le host en IP pour vérifier
	ips, err := net.LookupIP(host)
	if err != nil {
		// Si la résolution échoue, vérifier si c'est directement une IP
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("résolution DNS échouée pour %s: %w", host, err)
		}
		ips = []net.IP{ip}
	}

	for _, ip := range ips {
		if ip.IsLoopback() {
			return fmt.Errorf("adresse loopback interdite: %s → %s (protection SSRF)", host, ip)
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("adresse link-local interdite: %s → %s (protection SSRF)", host, ip)
		}
		if ip.IsMulticast() {
			return fmt.Errorf("adresse multicast interdite: %s → %s", host, ip)
		}
		if ip.IsUnspecified() {
			return fmt.Errorf("adresse non spécifiée interdite: %s → %s", host, ip)
		}
		// Bloquer le cloud metadata endpoint (AWS/GCP/Azure)
		if ip.Equal(net.ParseIP("169.254.169.254")) {
			return fmt.Errorf("adresse metadata cloud interdite: %s → %s (protection SSRF)", host, ip)
		}
	}

	return nil
}
