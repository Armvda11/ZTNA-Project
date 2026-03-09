package session

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// TestSessionRegistration tests registering new sessions
func TestSessionRegistration(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mgr := NewManager(log)

	session := &Session{
		Sub:          "auth0|user123",
		Username:     "alice",
		ResourceType: "ssh",
		ResourceHost: "backend.local",
		ResourcePort: 22,
		SourceIP:     "192.168.1.100",
		DecisionID:   "decision-456",
	}

	sessionID, err := mgr.Register(session)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if sessionID == "" {
		t.Error("Register() should return non-empty session ID")
	}

	// Verify session is tracked
	got, ok := mgr.GetSession(sessionID)
	if !ok || got == nil {
		t.Fatal("GetSession() returned nil for registered session")
	}
	if got.Sub != "auth0|user123" {
		t.Errorf("Session.Sub = %q, want %q", got.Sub, "auth0|user123")
	}
	if got.Username != "alice" {
		t.Errorf("Session.Username = %q, want %q", got.Username, "alice")
	}
	if mgr.ActiveCount() != 1 {
		t.Errorf("ActiveCount() = %d, want 1", mgr.ActiveCount())
	}
}

// TestSessionLimits tests per-subject connection limits
func TestSessionLimits(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mgr := NewManagerWithLimits(log, 5) // Low limit for testing

	sub := "auth0|user123"
	var registeredCount int
	for i := 0; i < 10; i++ {
		session := &Session{
			Sub:          sub,
			Username:     "alice",
			ResourceType: "tcp",
			ResourceHost: "backend.local",
			ResourcePort: 8080 + i,
		}
		_, err := mgr.Register(session)
		if err == nil {
			registeredCount++
		}
	}

	// Should have registered exactly maxPerSubject sessions
	if registeredCount != 5 {
		t.Errorf("Registered %d sessions, want exactly 5 (maxPerSubject limit)", registeredCount)
	}
	if mgr.ActiveCount() != 5 {
		t.Errorf("ActiveCount() = %d, want 5", mgr.ActiveCount())
	}
}

// TestSessionExpiration tests session TTL enforcement via GC
func TestSessionExpiration(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mgr := NewManager(log)
	mgr.gcInterval = 100 * time.Millisecond // Fast GC for testing

	session := &Session{
		Sub:          "auth0|user123",
		Username:     "alice",
		ResourceType: "ssh",
		ResourceHost: "backend.local",
		ResourcePort: 22,
		TTLSeconds:   1, // 1 second TTL
		DecisionID:   "decision-456",
		CancelFunc:   func() {}, // no-op cancel
	}

	sessionID, err := mgr.Register(session)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Session should have ExpiresAt set
	s, _ := mgr.GetSession(sessionID)
	if s.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set when TTLSeconds > 0")
	}

	// Wait for TTL to expire
	time.Sleep(1200 * time.Millisecond)

	// Start GC and let it tick
	ctx, cancel := context.WithCancel(context.Background())
	go mgr.StartGarbageCollector(ctx)
	time.Sleep(300 * time.Millisecond) // Let a GC tick pass
	cancel()

	// Session should have been reaped
	if mgr.ActiveCount() != 0 {
		t.Errorf("ActiveCount() = %d, want 0 (session should be expired)", mgr.ActiveCount())
	}
}

// TestSessionCleanup tests cleanup of closed sessions
func TestSessionCleanup(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mgr := NewManager(log)

	session := &Session{
		Sub:          "auth0|user123",
		Username:     "alice",
		ResourceType: "tcp",
		ResourceHost: "backend.local",
		ResourcePort: 8080,
	}

	sessionID, _ := mgr.Register(session)

	if mgr.ActiveCount() != 1 {
		t.Fatalf("ActiveCount() = %d, want 1 before unregister", mgr.ActiveCount())
	}

	// Unregister should remove the session
	mgr.Unregister(sessionID)

	if mgr.ActiveCount() != 0 {
		t.Errorf("ActiveCount() = %d, want 0 after Unregister", mgr.ActiveCount())
	}
	if _, ok := mgr.GetSession(sessionID); ok {
		t.Error("GetSession() should return nil after Unregister")
	}
}

// TestSessionMetrics tests session metrics collection
func TestSessionMetrics(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mgr := NewManager(log)

	// Register multiple sessions
	for i := 0; i < 5; i++ {
		session := &Session{
			Sub:          "auth0|user123",
			Username:     "alice",
			ResourceType: "tcp",
			ResourceHost: "backend.local",
			ResourcePort: 8080 + i,
		}
		_, err := mgr.Register(session)
		if err != nil {
			t.Fatalf("Register() session %d error = %v", i, err)
		}
	}

	if mgr.ActiveCount() != 5 {
		t.Errorf("ActiveCount() = %d, want 5", mgr.ActiveCount())
	}

	// ListActive should return all sessions
	active := mgr.ListActive()
	if len(active) != 5 {
		t.Errorf("ListActive() returned %d sessions, want 5", len(active))
	}

	// SetEndStats should update session metrics
	firstID := active[0].ID
	mgr.SetEndStats(firstID, 1024, 2048, "client_close")

	s, ok := mgr.GetSession(firstID)
	if !ok || s == nil {
		t.Fatal("GetSession() should not return nil before Unregister")
	}
	if s.BytesIn != 1024 {
		t.Errorf("BytesIn = %d, want 1024", s.BytesIn)
	}
	if s.BytesOut != 2048 {
		t.Errorf("BytesOut = %d, want 2048", s.BytesOut)
	}
	if s.EndReason != "client_close" {
		t.Errorf("EndReason = %q, want %q", s.EndReason, "client_close")
	}
}

// TestSessionConcurrentAccess tests thread-safe operations
func TestSessionConcurrentAccess(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mgr := NewManager(log)

	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	// Test: Multiple goroutines registering sessions simultaneously
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			session := &Session{
				Sub:          "auth0|user456",
				Username:     "bob",
				ResourceType: "tcp",
				ResourceHost: "backend.local",
				ResourcePort: 8080 + id,
			}
			_, err := mgr.Register(session)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Should handle concurrent access without panics
	// Max 10 per subject by default, so all 10 should succeed
	count := mgr.ActiveCount()
	if count != 10 {
		var errCount int
		for range errCh {
			errCount++
		}
		t.Errorf("ActiveCount() = %d, want 10 (errors: %d)", count, errCount)
	}

	// Test: Concurrent reads while writing
	var wg2 sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			_ = mgr.ListActive()
			_ = mgr.ActiveCount()
		}()
	}
	wg2.Wait()
}

// TestSessionKill tests admin kill functionality
func TestSessionKill(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mgr := NewManager(log)

	cancelled := false
	session := &Session{
		Sub:          "auth0|user123",
		Username:     "alice",
		ResourceType: "ssh",
		ResourceHost: "backend.local",
		ResourcePort: 22,
		CancelFunc:   func() { cancelled = true },
	}

	sessionID, _ := mgr.Register(session)

	// Kill should call CancelFunc and remove session
	mgr.KillSession(sessionID)

	if !cancelled {
		t.Error("KillSession() should call CancelFunc")
	}
	if mgr.ActiveCount() != 0 {
		t.Errorf("ActiveCount() = %d, want 0 after KillSession", mgr.ActiveCount())
	}
}
