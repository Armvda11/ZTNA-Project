package policy

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	domainPolicy "control-plane/internal/domain/policy"
	"control-plane/internal/domain/port"
)

// Service manages policy versions and delegates evaluation to the domain
// EvaluationEngine so the business logic remains testable without a database.
type Service struct {
	repo   port.PolicyRepository
	engine *domainPolicy.EvaluationEngine
}

func New(repo port.PolicyRepository) *Service {
	return &Service{
		repo:   repo,
		engine: domainPolicy.NewEvaluationEngine(),
	}
}

func (s *Service) CreateVersion(ctx context.Context, createdBy string, rules []model.PolicyRule) (int64, error) {
	for i := range rules {
		// Normalise to lowercase.
		rules[i].Effect = strings.ToLower(strings.TrimSpace(rules[i].Effect))
		rules[i].Action = strings.ToLower(rules[i].Action)
		rules[i].ResourceType = strings.ToLower(rules[i].ResourceType)

		// Validate effect: must be strictly "allow" or "deny".
		if err := domainPolicy.ValidateEffect(rules[i].Effect); err != nil {
			return 0, fmt.Errorf("rule %d: %w", i, err)
		}
	}

	return s.repo.CreatePolicyVersion(ctx, createdBy, rules)
}

func (s *Service) ActivateVersion(ctx context.Context, id int64) error {
	if err := s.repo.ActivatePolicyVersion(ctx, id); err != nil {
		if err == sql.ErrNoRows {
			return domainErrors.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Service) GetActive(ctx context.Context) (model.PolicySnapshot, error) {
	snapshot, err := s.repo.GetActivePolicy(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.PolicySnapshot{}, domainErrors.ErrNotFound
		}
		return model.PolicySnapshot{}, err
	}
	return snapshot, nil
}

// Evaluate delegates to the pure EvaluationEngine in the domain layer.
func (s *Service) Evaluate(
	snapshot model.PolicySnapshot,
	subject model.Subject,
	action string,
	resource model.Resource,
) (model.DecisionEffect, string) {
	return s.engine.Evaluate(snapshot, subject, action, resource)
}

// The matching helpers have been moved to internal/domain/policy/evaluation.go
// and are exercised via EvaluationEngine.Evaluate.
