package domain

import (
	"errors"
	"testing"
)

func TestErrorSentinels(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrAccessDenied", ErrAccessDenied, "accès refusé"},
		{"ErrInvalidCert", ErrInvalidCert, "certificat"},
		{"ErrNoIdentity", ErrNoIdentity, "identité"},
		{"ErrInvalidRequest", ErrInvalidRequest, "requête"},
		{"ErrTargetUnreachable", ErrTargetUnreachable, "ressource cible"},
		{"ErrControlPlaneUnreachable", ErrControlPlaneUnreachable, "control plane"},
		{"ErrTooManySessions", ErrTooManySessions, "sessions"},
		{"ErrSessionExpired", ErrSessionExpired, "session expirée"},
		{"ErrProtocolError", ErrProtocolError, "protocole"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s is nil", tt.name)
			}
			if tt.err.Error() == "" {
				t.Errorf("%s has empty error message", tt.name)
			}
			// Vérifier que le message contient au moins une partie attendue
			if tt.msg != "" && !contains(tt.err.Error(), tt.msg) {
				t.Errorf("%s.Error() = %q, should contain %q", tt.name, tt.err.Error(), tt.msg)
			}
		})
	}
}

func TestErrorWrapping(t *testing.T) {
	wrapped := errors.New("network failure")
	err := errors.Join(ErrControlPlaneUnreachable, wrapped)

	if !errors.Is(err, ErrControlPlaneUnreachable) {
		t.Error("errors.Is should detect ErrControlPlaneUnreachable")
	}
	if !errors.Is(err, wrapped) {
		t.Error("errors.Is should detect wrapped error")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
