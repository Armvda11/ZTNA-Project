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
		{"ErrNotAuthenticated", ErrNotAuthenticated, "non authentifié"},
		{"ErrTokenExpired", ErrTokenExpired, "token expiré"},
		{"ErrNoCertificate", ErrNoCertificate, "pas de certificat"},
		{"ErrCertExpired", ErrCertExpired, "certificat mTLS expiré"},
		{"ErrConnectionDenied", ErrConnectionDenied, "connexion refusée"},
		{"ErrGatewayUnreachable", ErrGatewayUnreachable, "gateway inaccessible"},
		{"ErrControlPlaneUnreachable", ErrControlPlaneUnreachable, "control plane inaccessible"},
		{"ErrInvalidConfig", ErrInvalidConfig, "configuration invalide"},
		{"ErrProtocolMismatch", ErrProtocolMismatch, "incompatibilité de protocole"},
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
	wrapped := errors.New("root cause")
	err := errors.Join(ErrConnectionDenied, wrapped)

	if !errors.Is(err, ErrConnectionDenied) {
		t.Error("errors.Is should detect ErrConnectionDenied")
	}
	if !errors.Is(err, wrapped) {
		t.Error("errors.Is should detect wrapped error")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
