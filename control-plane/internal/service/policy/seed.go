package policy

import (
	"context"
	"fmt"
	"os"

	"control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"

	"gopkg.in/yaml.v3"
)

type seedFile struct {
	CreatedBy string             `yaml:"created_by"`
	Rules     []model.PolicyRule `yaml:"rules"`
}

func (s *Service) SeedIfEmpty(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}

	_, err := s.GetActive(ctx)
	if err == nil {
		return nil
	}
	if err != errors.ErrNotFound {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read seed policy: %w", err)
	}

	var seed seedFile
	if err := yaml.Unmarshal(data, &seed); err != nil {
		return fmt.Errorf("parse seed policy: %w", err)
	}
	if len(seed.Rules) == 0 {
		return fmt.Errorf("seed policy has no rules")
	}
	createdBy := seed.CreatedBy
	if createdBy == "" {
		createdBy = "seed"
	}

	versionID, err := s.CreateVersion(ctx, createdBy, seed.Rules)
	if err != nil {
		return err
	}

	return s.ActivateVersion(ctx, versionID)
}
