// Package policy contains the pure evaluation engine for ZTNA policies.
// It has no database dependency and can be tested in isolation.
package policy

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
// The optional reqCtx carries contextual signals (e.g. device_trust, src_ip)
// that context-aware conditions in rules can evaluate against.
func (e *EvaluationEngine) Evaluate(
	snapshot model.PolicySnapshot,
	subject model.Subject,
	action string,
	resource model.Resource,
	reqCtx map[string]any,
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
		// Conditions contextuelles
		if rule.AllowedHours != "" && !matchAllowedHours(rule.AllowedHours) {
			continue
		}
		if rule.RequiredDeviceTrust != "" && !matchDeviceTrust(rule.RequiredDeviceTrust, reqCtx) {
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

// matchAllowedHours vérifie si l'heure courante est dans la plage autorisée.
// Format attendu : "HH:MM-HH:MM" (UTC). Ex: "08:00-18:00".
func matchAllowedHours(hoursSpec string) bool {
	parts := strings.SplitN(hoursSpec, "-", 2)
	if len(parts) != 2 {
		return true // format invalide → pas de restriction
	}
	now := time.Now().UTC()
	nowMinutes := now.Hour()*60 + now.Minute()

	start, err1 := parseHHMM(parts[0])
	end, err2 := parseHHMM(parts[1])
	if err1 != nil || err2 != nil {
		return true // format invalide → pas de restriction
	}

	if start <= end {
		return nowMinutes >= start && nowMinutes < end
	}
	// Plage à cheval sur minuit (ex: "22:00-06:00")
	return nowMinutes >= start || nowMinutes < end
}

func parseHHMM(s string) (int, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid HH:MM: %s", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, fmt.Errorf("invalid hour: %s", parts[0])
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid minute: %s", parts[1])
	}
	return h*60 + m, nil
}

// matchDeviceTrust vérifie que le device trust level du contexte est suffisant.
// Niveaux : low < medium < high. Si le contexte ne contient pas device_trust,
// la condition échoue (deny par défaut pour la sécurité).
func matchDeviceTrust(required string, reqCtx map[string]any) bool {
	if reqCtx == nil {
		return false
	}
	actual, ok := reqCtx["device_trust"].(string)
	if !ok || actual == "" {
		return false
	}
	return trustLevel(actual) >= trustLevel(required)
}

func trustLevel(level string) int {
	switch strings.ToLower(level) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
