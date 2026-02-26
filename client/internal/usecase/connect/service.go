// Package connect orchestre le workflow de connexion à une ressource distante
// via la Gateway ZTNA.
package connect

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"client/internal/core/domain"
	"client/internal/core/ports"
)
// Service implémente le usecase de connexion.
type Service struct {
	resources ports.ResourceCatalog
	certs     ports.CertificateStore
	tunnel    ports.TunnelConnector
	log       *slog.Logger
}

// NewService construit le usecase de connexion.
func NewService(resources ports.ResourceCatalog, certs ports.CertificateStore, tunnel ports.TunnelConnector, log *slog.Logger) *Service {
	return &Service{
		resources: resources,
		certs:     certs,
		tunnel:    tunnel,
		log:       log,
	}
}

// Run établit un tunnel mTLS vers la Gateway pour la ressource demandée,
// puis relaie le trafic entre le tunnel et stdin/stdout du processus.
func (s *Service) Run(ctx context.Context, resourceName string) error {
	resource, err := s.resources.Resolve(ctx, resourceName)
	if err != nil {
		return fmt.Errorf("résolution de ressource impossible: %w", err)
	}

	if err := resource.Validate(); err != nil {
		return fmt.Errorf("ressource invalide: %w", err)
	}

	certPEM, keyPEM, err := s.certs.LoadCertAndKey()
	if err != nil {
		return fmt.Errorf("%w: impossible de charger le certificat mTLS (%v)", domain.ErrNoCertificate, err)
	}

	tunnelConn, err := s.tunnel.Connect(certPEM, keyPEM, formatResource(resource))
	if err != nil {
		return fmt.Errorf("échec d'ouverture du tunnel vers la gateway: %w", err)
	}
	defer tunnelConn.Close()

	s.log.Info("tunnel mTLS établi, relais du trafic via stdin/stdout",
		"resource", resource.Name,
		"host", resource.Host,
		"port", resource.Port,
		"type", resource.Type,
	)

	// Relayer le trafic entre le tunnel mTLS et stdin/stdout
	stdinout := &stdinStdoutConn{}
	if err := s.tunnel.RelayTraffic(tunnelConn, stdinout); err != nil {
		s.log.Debug("relais terminé", "error", err)
	}
	return nil
}

// stdinStdoutConn implémente net.Conn en reliant os.Stdin et os.Stdout.
// Utilisé pour le mode CLI : le terminal de l'utilisateur devient
// le "local endpoint" du tunnel.
type stdinStdoutConn struct{}

func (s *stdinStdoutConn) Read(b []byte) (int, error)         { return os.Stdin.Read(b) }
func (s *stdinStdoutConn) Write(b []byte) (int, error)        { return os.Stdout.Write(b) }
func (s *stdinStdoutConn) Close() error                       { return nil }
func (s *stdinStdoutConn) LocalAddr() net.Addr                { return &net.UnixAddr{Name: "stdin"} }
func (s *stdinStdoutConn) RemoteAddr() net.Addr               { return &net.UnixAddr{Name: "stdout"} }
func (s *stdinStdoutConn) SetDeadline(t time.Time) error      { return nil }
func (s *stdinStdoutConn) SetReadDeadline(t time.Time) error  { return nil }
func (s *stdinStdoutConn) SetWriteDeadline(t time.Time) error { return nil }

func formatResource(resource domain.ResourceRef) string {
	if resource.Name != "" {
		return resource.Name
	}
	return fmt.Sprintf("%s://%s:%d", resource.Type, resource.Host, resource.Port)
}

// RunPortForward ouvre un listener TCP local sur 127.0.0.1:localPort et, pour
// chaque connexion entrante, établit un tunnel mTLS séparé vers la Gateway.
// Chaque connexion locale = 1 cycle complet ZTNA (mTLS + autorisation PEP).
//
// Usage typique :
//
//	ztna connect http:lan-app:80 --local-port 18080
//	# Dans un autre terminal :
//	curl http://127.0.0.1:18080/api/status
func (s *Service) RunPortForward(ctx context.Context, resourceName string, localPort int) error {
	resource, err := s.resources.Resolve(ctx, resourceName)
	if err != nil {
		return fmt.Errorf("résolution de ressource impossible: %w", err)
	}
	if err := resource.Validate(); err != nil {
		return fmt.Errorf("ressource invalide: %w", err)
	}

	certPEM, keyPEM, err := s.certs.LoadCertAndKey()
	if err != nil {
		return fmt.Errorf("%w: impossible de charger le certificat mTLS (%v)", domain.ErrNoCertificate, err)
	}

	listenAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("impossible d'ouvrir le port local %d: %w", localPort, err)
	}
	defer ln.Close()

	s.log.Info("port-forward actif — tunnel ZTNA prêt",
		"local_addr", listenAddr,
		"resource", resource.Name,
		"host", resource.Host,
		"port", resource.Port,
	)
	fmt.Printf("\033[0;32m[✓]\033[0m Port-forward actif : http://127.0.0.1:%d → %s (via ZTNA mTLS)\n", localPort, resourceName)
	fmt.Printf("    Appuyez sur Ctrl+C pour arrêter.\n")

	// Fermer le listener quand le contexte est annulé
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil // arrêt normal via signal
			}
			return fmt.Errorf("erreur accept: %w", err)
		}
		go s.handlePortForwardConn(ctx, conn, certPEM, keyPEM, formatResource(resource))
	}
}

func (s *Service) handlePortForwardConn(ctx context.Context, localConn net.Conn, certPEM, keyPEM []byte, resource string) {
	defer localConn.Close()
	_ = localConn.(*net.TCPConn).SetNoDelay(true) //nolint:errcheck

	tunnelConn, err := s.tunnel.Connect(certPEM, keyPEM, resource)
	if err != nil {
		s.log.Error("échec du tunnel pour connexion locale", "resource", resource, "error", err)
		return
	}
	defer tunnelConn.Close()

	if err := s.tunnel.RelayTraffic(tunnelConn, localConn); err != nil {
		s.log.Debug("relais port-forward terminé", "error", err)
	}
}
