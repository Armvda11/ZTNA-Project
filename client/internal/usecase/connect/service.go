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

func (s *stdinStdoutConn) Read(b []byte) (int, error)  { return os.Stdin.Read(b) }
func (s *stdinStdoutConn) Write(b []byte) (int, error) { return os.Stdout.Write(b) }
func (s *stdinStdoutConn) Close() error                { return nil }
func (s *stdinStdoutConn) LocalAddr() net.Addr         { return &net.UnixAddr{Name: "stdin"} }
func (s *stdinStdoutConn) RemoteAddr() net.Addr        { return &net.UnixAddr{Name: "stdout"} }
func (s *stdinStdoutConn) SetDeadline(t time.Time) error      { return nil }
func (s *stdinStdoutConn) SetReadDeadline(t time.Time) error  { return nil }
func (s *stdinStdoutConn) SetWriteDeadline(t time.Time) error { return nil }

func formatResource(resource domain.ResourceRef) string {
	if resource.Name != "" {
		return resource.Name
	}
	return fmt.Sprintf("%s://%s:%d", resource.Type, resource.Host, resource.Port)
}
