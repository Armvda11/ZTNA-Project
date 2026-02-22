package policy

import (
	"testing"

	"control-plane/internal/domain/model"
)

// helpers ─────────────────────────────────────────────────────

func snapshot(rules ...model.PolicyRule) model.PolicySnapshot {
	return model.PolicySnapshot{Rules: rules}
}

func rule(id int64, effect, subject, action, resType, resMatch string) model.PolicyRule {
	return model.PolicyRule{
		ID:            id,
		Effect:        effect,
		SubjectMatch:  subject,
		Action:        action,
		ResourceType:  resType,
		ResourceMatch: resMatch,
	}
}

func sshResource(host string, port int) model.Resource {
	return model.Resource{
		Type: model.ResourceSSH,
		SSH:  &model.SSHResource{Host: host, Port: port},
	}
}

func subject(username, sub string, groups ...string) model.Subject {
	return model.Subject{Username: username, Sub: sub, Groups: groups}
}

// ─────────────────────────────────────────────────────────────
// Evaluate tests
// ─────────────────────────────────────────────────────────────

func TestEvaluate_AllowOnFirstMatch(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(rule(1, "allow", "*", "ssh", "ssh", "*"))
	effect, reason := e.Evaluate(snap, subject("alice", "sub-1"), "ssh", sshResource("10.10.30.10", 22))
	if effect != model.DecisionAllow {
		t.Fatalf("expected allow, got %s (%s)", effect, reason)
	}
}

func TestEvaluate_DenyOnFirstMatch(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(rule(1, "deny", "*", "ssh", "ssh", "*"))
	effect, reason := e.Evaluate(snap, subject("bob", "sub-2"), "ssh", sshResource("10.10.30.10", 22))
	if effect != model.DecisionDeny {
		t.Fatalf("expected deny, got %s (%s)", effect, reason)
	}
}

func TestEvaluate_DefaultDenyWhenNoRuleMatches(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(rule(1, "allow", "user:alice", "ssh", "ssh", "*"))
	effect, reason := e.Evaluate(snap, subject("bob", "sub-2"), "ssh", sshResource("10.10.30.10", 22))
	if effect != model.DecisionDeny {
		t.Fatalf("expected default-deny, got %s", effect)
	}
	if reason != "default-deny" {
		t.Fatalf("expected reason 'default-deny', got %s", reason)
	}
}

func TestEvaluate_FirstMatchWins(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(
		rule(1, "deny", "user:alice", "ssh", "ssh", "*"),
		rule(2, "allow", "*", "ssh", "ssh", "*"),
	)
	effect, _ := e.Evaluate(snap, subject("alice", "sub-1"), "ssh", sshResource("10.10.30.10", 22))
	if effect != model.DecisionDeny {
		t.Fatalf("expected deny (first match), got %s", effect)
	}
}

func TestEvaluate_UserMatch(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(rule(1, "allow", "user:alice", "ssh", "ssh", "*"))
	if effect, _ := e.Evaluate(snap, subject("alice", "sub-1"), "ssh", sshResource("h", 22)); effect != model.DecisionAllow {
		t.Fatal("user:alice should match alice")
	}
	if effect, _ := e.Evaluate(snap, subject("bob", "sub-2"), "ssh", sshResource("h", 22)); effect != model.DecisionDeny {
		t.Fatal("user:alice should not match bob")
	}
}

func TestEvaluate_GroupMatch(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(rule(1, "allow", "group:admins", "ssh", "ssh", "*"))
	if effect, _ := e.Evaluate(snap, subject("alice", "sub-1", "admins", "devs"), "ssh", sshResource("h", 22)); effect != model.DecisionAllow {
		t.Fatal("group:admins should match member of admins")
	}
	if effect, _ := e.Evaluate(snap, subject("bob", "sub-2", "devs"), "ssh", sshResource("h", 22)); effect != model.DecisionDeny {
		t.Fatal("group:admins should not match non-member")
	}
}

func TestEvaluate_SubMatch(t *testing.T) {
	e := NewEvaluationEngine()
	sub := "abc-123"
	snap := snapshot(rule(1, "allow", "sub:"+sub, "ssh", "ssh", "*"))
	if effect, _ := e.Evaluate(snap, subject("alice", sub), "ssh", sshResource("h", 22)); effect != model.DecisionAllow {
		t.Fatal("sub: should match exact sub")
	}
}

func TestEvaluate_WildcardResourceMatch(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(rule(1, "allow", "*", "ssh", "ssh", "ssh:10.10.30.*"))
	if effect, _ := e.Evaluate(snap, subject("u", "s"), "ssh", sshResource("10.10.30.10", 22)); effect != model.DecisionAllow {
		t.Fatal("prefix wildcard should match")
	}
	if effect, _ := e.Evaluate(snap, subject("u", "s"), "ssh", sshResource("10.10.20.10", 22)); effect != model.DecisionDeny {
		t.Fatal("prefix wildcard should not match different subnet")
	}
}

func TestEvaluate_ExactResourceMatch(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(rule(1, "allow", "*", "ssh", "ssh", "ssh:10.10.30.10:22"))
	if effect, _ := e.Evaluate(snap, subject("u", "s"), "ssh", sshResource("10.10.30.10", 22)); effect != model.DecisionAllow {
		t.Fatal("exact match should match")
	}
	if effect, _ := e.Evaluate(snap, subject("u", "s"), "ssh", sshResource("10.10.30.99", 22)); effect != model.DecisionDeny {
		t.Fatal("exact match should not match different host")
	}
}

func TestEvaluate_ActionMismatch(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(rule(1, "allow", "*", "rdp", "ssh", "*"))
	if effect, _ := e.Evaluate(snap, subject("u", "s"), "ssh", sshResource("h", 22)); effect != model.DecisionDeny {
		t.Fatal("action mismatch should result in default-deny")
	}
}

func TestEvaluate_ResourceTypeMismatch(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot(rule(1, "allow", "*", "ssh", "rdp", "*"))
	if effect, _ := e.Evaluate(snap, subject("u", "s"), "ssh", sshResource("h", 22)); effect != model.DecisionDeny {
		t.Fatal("resource type mismatch should result in default-deny")
	}
}

func TestEvaluate_EmptySnapshot(t *testing.T) {
	e := NewEvaluationEngine()
	snap := snapshot()
	effect, reason := e.Evaluate(snap, subject("u", "s"), "ssh", sshResource("h", 22))
	if effect != model.DecisionDeny || reason != "default-deny" {
		t.Fatalf("empty snapshot should default-deny, got %s/%s", effect, reason)
	}
}

// ─────────────────────────────────────────────────────────────
// ValidateEffect tests
// ─────────────────────────────────────────────────────────────

func TestValidateEffect(t *testing.T) {
	cases := []struct {
		input   string
		wantErr bool
	}{
		{"allow", false},
		{"deny", false},
		{"ALLOW", false},
		{"Deny", false},
		{"  allow  ", false},
		{"", true},
		{"permit", true},
		{"block", true},
		{"allow deny", true},
	}
	for _, tc := range cases {
		err := ValidateEffect(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateEffect(%q): wantErr=%v, got err=%v", tc.input, tc.wantErr, err)
		}
	}
}
