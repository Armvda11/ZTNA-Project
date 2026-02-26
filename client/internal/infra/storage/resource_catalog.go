package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"client/internal/config"
	"client/internal/core/domain"
)

const resourceCatalogFileName = "resources.json"

// ResourceCatalogFile résout des ressources à partir d'un nom logique,
// d'une URI directe ou d'un host:port explicite.
//
// Il facilite les communications inter-machines dans le lab/prod sans
// hardcoder les adresses dans le code.
type ResourceCatalogFile struct {
	cfg *config.Config
	log *slog.Logger
}

// NewResourceCatalogFile construit un catalog basé sur storage.path/resources.json.
func NewResourceCatalogFile(cfg *config.Config, log *slog.Logger) *ResourceCatalogFile {
	return &ResourceCatalogFile{cfg: cfg, log: log}
}

// Resolve traduit resourceName en ResourceRef.
//
// Formats supportés:
//   - Nom logique: "backend-ssh" (résolution via resources.json)
//   - URI: "ssh://10.0.20.10:22"
//   - Endpoint brut: "10.0.20.10:22"
func (c *ResourceCatalogFile) Resolve(_ context.Context, resourceName string) (domain.ResourceRef, error) {
	if resourceName == "" {
		return domain.ResourceRef{}, domain.ErrInvalidConfig
	}

	if strings.Contains(resourceName, "://") {
		return parseURIResource(resourceName)
	}

	if host, port, err := net.SplitHostPort(resourceName); err == nil {
		portValue, parseErr := strconv.Atoi(port)
		if parseErr != nil {
			return domain.ResourceRef{}, fmt.Errorf("port invalide: %w", parseErr)
		}
		return domain.ResourceRef{Type: "tcp", Host: host, Port: portValue, Name: resourceName}, nil
	}

	resources, err := c.loadCatalog()
	if err != nil {
		return domain.ResourceRef{}, err
	}

	ref, ok := resources[resourceName]
	if !ok {
		return domain.ResourceRef{}, fmt.Errorf("ressource inconnue: %s", resourceName)
	}

	if err := ref.Validate(); err != nil {
		return domain.ResourceRef{}, fmt.Errorf("ressource invalide (%s): %w", resourceName, err)
	}

	if ref.Name == "" {
		ref.Name = resourceName
	}

	return ref, nil
}

func (c *ResourceCatalogFile) loadCatalog() (map[string]domain.ResourceRef, error) {
	path := filepath.Join(c.cfg.Storage.Path, resourceCatalogFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("catalogue de ressources absent (%s): ajoutez ce fichier ou utilisez une URI directe", path)
		}
		return nil, fmt.Errorf("impossible de lire le catalogue de ressources: %w", err)
	}

	resources := make(map[string]domain.ResourceRef)
	if err := json.Unmarshal(data, &resources); err != nil {
		return nil, fmt.Errorf("catalogue de ressources invalide: %w", err)
	}

	c.log.Debug("catalogue de ressources chargé", "path", path, "entries", len(resources))
	return resources, nil
}

func parseURIResource(raw string) (domain.ResourceRef, error) {
	parts := strings.SplitN(raw, "://", 2)
	if len(parts) != 2 {
		return domain.ResourceRef{}, fmt.Errorf("URI ressource invalide: %s", raw)
	}

	resourceType := strings.ToLower(strings.TrimSpace(parts[0]))
	if resourceType == "" {
		return domain.ResourceRef{}, fmt.Errorf("type de ressource vide")
	}

	host, portString, err := net.SplitHostPort(parts[1])
	if err != nil {
		return domain.ResourceRef{}, fmt.Errorf("endpoint URI invalide: %w", err)
	}

	port, err := strconv.Atoi(portString)
	if err != nil {
		return domain.ResourceRef{}, fmt.Errorf("port URI invalide: %w", err)
	}

	ref := domain.ResourceRef{
		Type: resourceType,
		Host: host,
		Port: port,
		Name: raw,
	}
	if err := ref.Validate(); err != nil {
		return domain.ResourceRef{}, err
	}

	return ref, nil
}
