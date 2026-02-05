package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ztna/control-plane/internal/config"
	"github.com/ztna/control-plane/internal/logger"
)

// Storage handles database operations
type Storage struct {
	db     *sql.DB
	logger *logger.Logger
}

// User represents a user in the system
type User struct {
	ID           int64
	Username     string
	PasswordHash string // bcrypt hash
	Email        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID        int64
	Timestamp time.Time
	Username  string
	Action    string
	Resource  string
	Result    string
	IPAddress string
	Details   string
}

// New creates a new storage instance
func New(cfg config.DatabaseConfig, log *logger.Logger) (*Storage, error) {
	var db *sql.DB
	var err error

	switch cfg.Type {
	case "sqlite":
		db, err = sql.Open("sqlite3", cfg.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite database: %w", err)
		}
	case "postgres":
		db, err = sql.Open("postgres", cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres database: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &Storage{
		db:     db,
		logger: log,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates database tables if they don't exist
func (s *Storage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		email TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		username TEXT NOT NULL,
		action TEXT NOT NULL,
		resource TEXT,
		result TEXT NOT NULL,
		ip_address TEXT,
		details TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_audit_username ON audit_logs(username);
	CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Create default users if table is empty
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return err
	}

	if count == 0 {
		s.logger.Info("Creating default users")
		defaultUsers := []struct {
			username string
			password string
		}{
			{"alice", "alice123"},
			{"bob", "bob123"},
		}

		for _, u := range defaultUsers {
			if _, err := s.CreateUser(u.username, u.password, ""); err != nil {
				s.logger.Warn("Failed to create default user", "username", u.username, "error", err)
			}
		}
	}

	return nil
}

// CreateUser creates a new user with a bcrypt password hash.
func (s *Storage) CreateUser(username, password, email string) (*User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := s.db.Exec(
		"INSERT INTO users (username, password_hash, email) VALUES (?, ?, ?)",
		username, string(passwordHash), email,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, _ := result.LastInsertId()
	s.logger.Info("Created user", "username", username, "id", id)

	return &User{
		ID:       id,
		Username: username,
		Email:    email,
	}, nil
}

// GetUserByUsername retrieves a user by username
func (s *Storage) GetUserByUsername(username string) (*User, error) {
	var user User
	err := s.db.QueryRow(
		"SELECT id, username, password_hash, email, created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// ValidatePassword validates a user's password and upgrades legacy plaintext hashes.
func (s *Storage) ValidatePassword(username, password string) (*User, error) {
	user, err := s.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}

	if isBcryptHash(user.PasswordHash) {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			return nil, fmt.Errorf("invalid password")
		}
		return user, nil
	}

	// Legacy plaintext comparison for existing users, then upgrade to bcrypt.
	if user.PasswordHash != password {
		return nil, fmt.Errorf("invalid password")
	}

	if err := s.updatePasswordHash(user.ID, password); err != nil {
		s.logger.Warn("Failed to upgrade password hash", "username", username, "error", err)
	}

	return user, nil
}

func (s *Storage) updatePasswordHash(userID int64, password string) error {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if _, err := s.db.Exec(
		"UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		string(passwordHash), userID,
	); err != nil {
		return fmt.Errorf("failed to update password hash: %w", err)
	}

	return nil
}

func isBcryptHash(hash string) bool {
	return strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$")
}

// LogAudit creates an audit log entry
func (s *Storage) LogAudit(username, action, resource, result, ipAddress, details string) error {
	_, err := s.db.Exec(
		"INSERT INTO audit_logs (username, action, resource, result, ip_address, details) VALUES (?, ?, ?, ?, ?, ?)",
		username, action, resource, result, ipAddress, details,
	)
	if err != nil {
		return fmt.Errorf("failed to log audit: %w", err)
	}

	s.logger.Debug("Audit logged",
		"username", username,
		"action", action,
		"resource", resource,
		"result", result,
	)

	return nil
}

// GetAuditLogs retrieves recent audit logs
func (s *Storage) GetAuditLogs(limit int) ([]AuditLog, error) {
	rows, err := s.db.Query(
		"SELECT id, timestamp, username, action, resource, result, ip_address, details FROM audit_logs ORDER BY timestamp DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		if err := rows.Scan(&log.ID, &log.Timestamp, &log.Username, &log.Action, &log.Resource, &log.Result, &log.IPAddress, &log.Details); err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}
