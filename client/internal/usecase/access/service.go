// Package access implémente le cas d'usage unifié « accéder à une ressource
// publiée par nom ». Il ouvre un tunnel mTLS vers la Gateway puis écoute en
// local pour relayer le trafic (port forward pour web/db, proxy SSH).
package access

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"client/internal/core/domain"
	"client/internal/core/ports"
)

// Service orchestre l'accès à une ressource publiée.
type Service struct {
	certs  ports.CertificateStore
	tunnel ports.TunnelConnector
	log    *slog.Logger
}

// NewService crée un nouveau service d'accès aux ressources.
func NewService(certs ports.CertificateStore, tunnel ports.TunnelConnector, log *slog.Logger) *Service {
	return &Service{certs: certs, tunnel: tunnel, log: log}
}

// Run ouvre un tunnel vers la ressource nommée et démarre un port forward local.
// Le port local est retourné via le callback onReady. La fonction bloque jusqu'à
// la fin du contexte ou la fermeture de la connexion.
func (s *Service) Run(ctx context.Context, resourceName string, localAddr string) error {
	certPEM, keyPEM, err := s.certs.LoadCertAndKey()
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrNoCertificate, err)
	}

	// Ouvrir le listener local
	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("impossible d'écouter sur %s: %w", localAddr, err)
	}
	defer ln.Close()

	s.log.Info("port forward prêt",
		"resource", resourceName,
		"local_addr", ln.Addr().String(),
	)
	fmt.Printf("→ Ressource '%s' disponible sur %s\n", resourceName, ln.Addr().String())
	fmt.Println("  Ctrl+C pour fermer le tunnel.")

	// Accepter les connexions locales, une à la fois pour simplifier
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		localConn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("erreur accept: %w", err)
			}
		}

		go func(local net.Conn) {
			defer local.Close()
			tunnelConn, err := s.tunnel.Connect(certPEM, keyPEM, resourceName)
			if err != nil {
				s.log.Error("échec ouverture tunnel", "resource", resourceName, "error", err)
				return
			}
			defer tunnelConn.Close()
			s.log.Info("session ouverte", "resource", resourceName, "local", local.RemoteAddr())
			relay(tunnelConn, local)
			s.log.Info("session fermée", "resource", resourceName)
		}(localConn)
	}
}

// relay copie le trafic bidirectionnellement entre deux connexions.
func relay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	copy := func(dst, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)
	}
	go copy(a, b)
	go copy(b, a)
	wg.Wait()
}