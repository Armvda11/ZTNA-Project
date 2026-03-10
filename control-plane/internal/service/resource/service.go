package resource

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"

	"control-plane/internal/domain/model"
	"control-plane/internal/domain/port"

	"gopkg.in/yaml.v3"
)

// Service manages published resources.
type Service struct {
	repo port.ResourceRepository
}

// New creates a new resource service.
func New(repo port.ResourceRepository) *Service {
	return &Service{repo: repo}
}

// Create registers a new published resource.
func (s *Service) Create(ctx context.Context, res model.PublishedResource) error {
	if res.ID == "" {
		res.ID = generateID()
	}
	return s.repo.CreateResource(ctx, res)
}

// Update modifies an existing published resource.
func (s *Service) Update(ctx context.Context, res model.PublishedResource) error {
	return s.repo.UpdateResource(ctx, res)
}

// GetByName returns a resource by its unique slug name.
func (s *Service) GetByName(ctx context.Context, name string) (model.PublishedResource, error) {
	return s.repo.GetResourceByName(ctx, name)
}

// List returns all published resources.
func (s *Service) List(ctx context.Context) ([]model.PublishedResource, error) {
	return s.repo.ListResources(ctx)
}

// ListForGroups returns resources visible to the given groups.
func (s *Service) ListForGroups(ctx context.Context, groups []string) ([]model.PublishedResource, error) {
	return s.repo.ListResourcesForGroups(ctx, groups)
}

// Delete removes a published resource.
func (s *Service) Delete(ctx context.Context, name string) error {
	return s.repo.DeleteResource(ctx, name)
}

// SeedIfEmpty loads resources from a YAML seed file when the table is empty.
func (s *Service) SeedIfEmpty(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}

	existing, err := s.repo.ListResources(ctx)
	if err != nil {
		return fmt.Errorf("check existing resources: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read resource seed: %w", err)
	}

	var seed seedResourceFile
	if err := yaml.Unmarshal(data, &seed); err != nil {
		return fmt.Errorf("parse resource seed: %w", err)
	}

	for _, res := range seed.Resources {
		if res.ID == "" {
			res.ID = generateID()
		}
		// Skip duplicates gracefully.
		if _, err := s.repo.GetResourceByName(ctx, res.Name); err == nil {
			continue
		} else if err != sql.ErrNoRows {
			return fmt.Errorf("check resource %s: %w", res.Name, err)
		}
		if err := s.repo.CreateResource(ctx, res); err != nil {
			return fmt.Errorf("seed resource %s: %w", res.Name, err)
		}
	}
	return nil
}

type seedResourceFile struct {
	Resources []model.PublishedResource `yaml:"resources"`
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}