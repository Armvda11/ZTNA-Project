package policy

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/store/sqlite"
)

type Service struct {
	store *sqlite.Store
}

func New(store *sqlite.Store) *Service {
	return &Service{store: store}
}

func (s *Service) CreateVersion(ctx context.Context, createdBy string, rules []model.PolicyRule) (int64, error) {
	for i := range rules {
		rules[i].Effect = strings.ToLower(rules[i].Effect)
		rules[i].Action = strings.ToLower(rules[i].Action)
		rules[i].ResourceType = strings.ToLower(rules[i].ResourceType)
	}

	return s.store.CreatePolicyVersion(ctx, createdBy, rules)
}

func (s *Service) ActivateVersion(ctx context.Context, id int64) error {
	if err := s.store.ActivatePolicyVersion(ctx, id); err != nil {
		if err == sql.ErrNoRows {
			return errors.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Service) GetActive(ctx context.Context) (model.PolicySnapshot, error) {
	snapshot, err := s.store.GetActivePolicy(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.PolicySnapshot{}, errors.ErrNotFound
		}
		return model.PolicySnapshot{}, err
	}
	return snapshot, nil
}

func (s *Service) Evaluate(snapshot model.PolicySnapshot, subject model.Subject, action string, resource model.Resource) (model.DecisionEffect, string) {
	canonicalResource := resource.Canonical()
	for _, rule := range snapshot.Rules {
		if !matchSubject(rule.SubjectMatch, subject) {
			continue
		}
		if !matchAction(rule.Action, action) {
			continue
		}
		if !matchResourceType(rule.ResourceType, string(resource.Type)) {
			continue
		}
		if !matchResource(rule.ResourceMatch, canonicalResource) {
			continue
		}
		reason := fmt.Sprintf("rule:%d", rule.ID)
		if rule.Effect == "allow" {
			return model.DecisionAllow, reason
		}
		return model.DecisionDeny, reason
	}
	return model.DecisionDeny, "default-deny"
}

func matchSubject(pattern string, subject model.Subject) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "user:") {
		return subject.Username == strings.TrimPrefix(pattern, "user:")
	}
	if strings.HasPrefix(pattern, "group:") {
		group := strings.TrimPrefix(pattern, "group:")
		for _, value := range subject.Groups {
			if value == group {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(pattern, "sub:") {
		return subject.Sub == strings.TrimPrefix(pattern, "sub:")
	}
	return false
}

func matchAction(pattern string, action string) bool {
	if pattern == "*" {
		return true
	}
	return strings.EqualFold(pattern, action)
}

func matchResourceType(pattern string, resourceType string) bool {
	if pattern == "*" {
		return true
	}
	return strings.EqualFold(pattern, resourceType)
}

func matchResource(pattern string, canonical string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(canonical, prefix)
	}
	return pattern == canonical
}
