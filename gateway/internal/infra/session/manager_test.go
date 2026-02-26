package session

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mgr := NewManager(log)

	if mgr == nil {
		t.Fatal("NewManager() returned nil")
	}
	if mgr.log != log {
		t.Error("NewManager() did not store logger")
	}
	if mgr.sessions == nil {
		t.Error("NewManager() did not initialize sessions map")
	}
}

func TestSession_Structure(t *testing.T) {
	s := &Session{
		ID:           "session-123",
		Sub:          "auth0|user789",
		Username:     "charlie",
		ResourceType: "ssh",
		ResourceHost: "10.0.1.50",
		ResourcePort: 22,
		StartedAt:    time.Now(),
		SourceIP:     "192.168.1.100",
		DecisionID:   "decision-456",
	}

	if s.ID != "session-123" {
		t.Errorf("ID = %q, want %q", s.ID, "session-123")
	}
	if s.Sub != "auth0|user789" {
		t.Errorf("Sub = %q, want %q", s.Sub, "auth0|user789")
	}
	if s.ResourceType != "ssh" {
		t.Errorf("ResourceType = %q, want %q", s.ResourceType, "ssh")
	}
	if s.ResourcePort != 22 {
		t.Errorf("ResourcePort = %d, want %d", s.ResourcePort, 22)
	}
}
