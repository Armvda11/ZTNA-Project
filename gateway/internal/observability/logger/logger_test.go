package logger

import (
	"context"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		level  string
		format string
		want   bool
	}{
		{"debug json", "debug", "json", true},
		{"info text", "info", "text", true},
		{"warn json", "warn", "json", true},
		{"error text", "error", "text", true},
		{"invalid level defaults to info", "invalid", "json", true},
		{"empty format defaults to json", "info", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := New(tt.level, tt.format)
			if (log != nil) != tt.want {
				t.Errorf("New() = %v, want non-nil: %v", log, tt.want)
			}
		})
	}
}

func TestWithRequestID(t *testing.T) {
	ctx := context.Background()
	requestID := "test-request-456"

	ctx = WithRequestID(ctx, requestID)
	got := RequestIDFromContext(ctx)

	if got != requestID {
		t.Errorf("RequestIDFromContext() = %q, want %q", got, requestID)
	}
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	got := RequestIDFromContext(ctx)

	if got != "" {
		t.Errorf("RequestIDFromContext() = %q, want empty string", got)
	}
}

func TestWithSessionID(t *testing.T) {
	ctx := context.Background()
	sessionID := "session-789"

	ctx = WithSessionID(ctx, sessionID)
	got := SessionIDFromContext(ctx)

	if got != sessionID {
		t.Errorf("SessionIDFromContext() = %q, want %q", got, sessionID)
	}
}

func TestSessionIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	got := SessionIDFromContext(ctx)

	if got != "" {
		t.Errorf("SessionIDFromContext() = %q, want empty string", got)
	}
}

func TestWithSubject(t *testing.T) {
	ctx := context.Background()
	subject := "auth0|user123"

	ctx = WithSubject(ctx, subject)
	got := SubjectFromContext(ctx)

	if got != subject {
		t.Errorf("SubjectFromContext() = %q, want %q", got, subject)
	}
}

func TestSubjectFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	got := SubjectFromContext(ctx)

	if got != "" {
		t.Errorf("SubjectFromContext() = %q, want empty string", got)
	}
}
