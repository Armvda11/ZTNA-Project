// Package connect orchestre le workflow de connexion à une ressource distante
// via la Gateway ZTNA.
package connect

import (
	"context"
	"fmt"
	"log/slog"

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

// Run établit un tunnel mTLS vers la Gateway pour la ressource demandée.
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

	// Le relais de trafic (stdin/stdout, port-forward, proxy HTTP local) sera
	// câblé ultérieurement selon le mode d'utilisation choisi par le CLI.
	// Pour l'instant, le tunnel est établi et la décision Gateway validée.
	s.log.Info("tunnel mTLS établi avec succès",
		"resource", resource.Name,
		"host", resource.Host,
		"port", resource.Port,
		"type", resource.Type,
	)
	return nil
}

func formatResource(resource domain.ResourceRef) string {
	if resource.Name != "" {
		return resource.Name
	}
	return fmt.Sprintf("%s://%s:%d", resource.Type, resource.Host, resource.Port)
}
