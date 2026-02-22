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
	requestID := "test-request-123"

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

func TestWithResource(t *testing.T) {
	ctx := context.Background()
	resource := "ssh://backend-server:22"

	ctx = WithResource(ctx, resource)
	got := ResourceFromContext(ctx)

	if got != resource {
		t.Errorf("ResourceFromContext() = %q, want %q", got, resource)
	}
}

func TestResourceFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	got := ResourceFromContext(ctx)

	if got != "" {
		t.Errorf("ResourceFromContext() = %q, want empty string", got)
	}
}
