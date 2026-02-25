// Package policy contains the pure evaluation engine for ZTNA policies.
// It has no database dependency and can be tested in isolation.
package policy

import (
	"fmt"
	"strings"

	"control-plane/internal/domain/model"
)

// EvaluationEngine evaluates a PolicySnapshot against an access request.
type EvaluationEngine struct{}

// NewEvaluationEngine creates a new evaluation engine.
func NewEvaluationEngine() *EvaluationEngine {
	return &EvaluationEngine{}
}

// Evaluate returns the decision effect and the human-readable reason for
// the first matching rule. If no rule matches, a default-deny is returned.
func (e *EvaluationEngine) Evaluate(
	snapshot model.PolicySnapshot,
	subject model.Subject,
	action string,
	resource model.Resource,
) (model.DecisionEffect, string) {
	canonical := resource.Canonical()
	for _, rule := range snapshot.Rules {
		if !matchSubject(rule.SubjectMatch, subject) {
			continue
		}
		if !matchAction(rule.Action, action) {
			continue
		}
		if !matchField(rule.ResourceType, string(resource.Type)) {
			continue
		}
		if !matchResource(rule.ResourceMatch, canonical) {
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

// ValidateEffect returns an error if effect is not "allow" or "deny".
func ValidateEffect(effect string) error {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "allow", "deny":
		return nil
	default:
		return fmt.Errorf("invalid effect %q: must be allow or deny", effect)
	}
}

// ──────────────────────────────────────────────────────────────
// Internal matching helpers
// ──────────────────────────────────────────────────────────────

func matchSubject(pattern string, subject model.Subject) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "user:") {
		return subject.Username == strings.TrimPrefix(pattern, "user:")
	}
	if strings.HasPrefix(pattern, "group:") {
		group := strings.TrimPrefix(pattern, "group:")
		for _, g := range subject.Groups {
			if g == group {
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

func matchAction(pattern, action string) bool {
	if pattern == "*" {
		return true
	}
	return strings.EqualFold(pattern, action)
}

func matchField(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	return strings.EqualFold(pattern, value)
}

func matchResource(pattern, canonical string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(canonical, prefix)
	}
	return strings.EqualFold(pattern, canonical)
}
