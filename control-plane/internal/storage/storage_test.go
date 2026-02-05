package storage

import (
	"testing"

	"github.com/ztna/control-plane/internal/config"
	"github.com/ztna/control-plane/internal/logger"
)

func setupTestStorage(t *testing.T) *Storage {
	cfg := config.DatabaseConfig{
		Type: "sqlite",
		Path: ":memory:",
	}

	log := logger.New(config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stderr",
	})

	store, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	return store
}

func TestCreateUser(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	user, err := store.CreateUser("testuser", "testpass", "test@example.com")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}

	if user.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", user.Email)
	}
}

func TestGetUserByUsername(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	// alice already exists from default users, so just get it
	user, err := store.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if user.Username != "alice" {
		t.Errorf("Expected username 'alice', got '%s'", user.Username)
	}

	// Try non-existent user
	_, err = store.GetUserByUsername("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent user, got nil")
	}
}

func TestValidatePassword(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	// bob already exists from default users with password "bob123"
	// Valid password
	user, err := store.ValidatePassword("bob", "bob123")
	if err != nil {
		t.Errorf("Expected valid password, got error: %v", err)
	}
	if user.Username != "bob" {
		t.Errorf("Expected username 'bob', got '%s'", user.Username)
	}

	// Invalid password
	_, err = store.ValidatePassword("bob", "wrongpassword")
	if err == nil {
		t.Error("Expected error for invalid password, got nil")
	}
}

func TestLogAudit(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	err := store.LogAudit("alice", "login", "", "success", "192.168.1.1", "test login")
	if err != nil {
		t.Fatalf("Failed to log audit: %v", err)
	}

	// Get logs
	logs, err := store.GetAuditLogs(10)
	if err != nil {
		t.Fatalf("Failed to get audit logs: %v", err)
	}

	if len(logs) == 0 {
		t.Error("Expected at least one audit log")
	}

	log := logs[0]
	if log.Username != "alice" {
		t.Errorf("Expected username 'alice', got '%s'", log.Username)
	}
	if log.Action != "login" {
		t.Errorf("Expected action 'login', got '%s'", log.Action)
	}
}

func TestDefaultUsers(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	// Check default users exist
	defaultUsers := []string{"alice", "bob"}
	for _, username := range defaultUsers {
		user, err := store.GetUserByUsername(username)
		if err != nil {
			t.Errorf("Default user '%s' not found: %v", username, err)
		}
		if user.Username != username {
			t.Errorf("Expected username '%s', got '%s'", username, user.Username)
		}
		// Verify password is bcrypt hash, not plaintext
		if !isBcryptHash(user.PasswordHash) {
			t.Errorf("Expected bcrypt hash for user '%s', got: %s", username, user.PasswordHash)
		}
	}
}

func TestPlaintextPasswordMigration(t *testing.T) {
	// This test verifies the automatic migration from plaintext to bcrypt hashes
	store := setupTestStorage(t)
	defer store.Close()

	// Manually insert a plaintext password (simulating legacy data)
	plainPassword := "legacypass123"
	_, err := store.db.Exec(
		"INSERT INTO users (username, password_hash, email) VALUES (?, ?, ?)",
		"legacyuser", plainPassword, "legacy@example.com",
	)
	if err != nil {
		t.Fatalf("Failed to insert legacy user: %v", err)
	}

	// First validation - should detect plaintext and auto-upgrade
	user, err := store.ValidatePassword("legacyuser", plainPassword)
	if err != nil {
		t.Errorf("Expected successful validation of plaintext password, got error: %v", err)
	}
	if user.Username != "legacyuser" {
		t.Errorf("Expected username 'legacyuser', got '%s'", user.Username)
	}

	// Verify the hash was upgraded to bcrypt
	updatedUser, err := store.GetUserByUsername("legacyuser")
	if err != nil {
		t.Fatalf("Failed to get updated user: %v", err)
	}
	if !isBcryptHash(updatedUser.PasswordHash) {
		t.Errorf("Expected bcrypt hash after migration, got: %s", updatedUser.PasswordHash)
	}

	// Second validation - should now use bcrypt comparison
	user, err = store.ValidatePassword("legacyuser", plainPassword)
	if err != nil {
		t.Errorf("Expected successful validation after migration, got error: %v", err)
	}
}
