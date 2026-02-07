package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	Role         string
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

// PolicyVersion represents a policy version snapshot
type PolicyVersion struct {
	ID          int64     `json:"id"`
	Description string    `json:"description"`
	DefaultDeny bool      `json:"default_deny"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

// PolicyRule represents a policy rule
type PolicyRule struct {
	ID          int64     `json:"id"`
	VersionID   int64     `json:"version_id"`
	SubjectType string    `json:"subject_type"`
	Subject     string    `json:"subject"`
	Resource    string    `json:"resource"`
	Allowed     bool      `json:"allowed"`
	CreatedAt   time.Time `json:"created_at"`
}

// PolicyRuleInput represents a policy rule input
type PolicyRuleInput struct {
	SubjectType string `json:"subject_type"`
	Subject     string `json:"subject"`
	Resource    string `json:"resource"`
	Allowed     bool   `json:"allowed"`
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
		role TEXT DEFAULT 'user',
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

	CREATE TABLE IF NOT EXISTS refresh_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		token_hash TEXT UNIQUE NOT NULL,
		username TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		revoked_at DATETIME
	);

	CREATE INDEX IF NOT EXISTS idx_refresh_username ON refresh_tokens(username);
	CREATE INDEX IF NOT EXISTS idx_refresh_expires ON refresh_tokens(expires_at);

	CREATE TABLE IF NOT EXISTS revoked_tokens (
		jti TEXT PRIMARY KEY,
		revoked_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS policy_versions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		description TEXT,
		default_deny BOOLEAN NOT NULL,
		active BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS policy_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		subject_type TEXT NOT NULL,
		subject TEXT NOT NULL,
		resource TEXT NOT NULL,
		allowed BOOLEAN NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (version_id) REFERENCES policy_versions(id)
	);

	CREATE INDEX IF NOT EXISTS idx_policy_rules_version ON policy_rules(version_id);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Best-effort migration for role column
	if _, err := s.db.Exec("ALTER TABLE users ADD COLUMN role TEXT DEFAULT 'user'"); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") && !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to add users.role column: %w", err)
		}
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
			role     string
		}{
			{"alice", "alice123", "admin"},
			{"bob", "bob123", "user"},
		}

		for _, u := range defaultUsers {
			if _, err := s.CreateUserWithRole(u.username, u.password, "", u.role); err != nil {
				s.logger.Warn("Failed to create default user", "username", u.username, "error", err)
			}
		}
	}

	return nil
}

// CreateUser creates a new user with a bcrypt password hash.
func (s *Storage) CreateUser(username, password, email string) (*User, error) {
	return s.CreateUserWithRole(username, password, email, "user")
}

// CreateUserWithRole creates a new user with a bcrypt password hash and role.
func (s *Storage) CreateUserWithRole(username, password, email, role string) (*User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := s.db.Exec(
		"INSERT INTO users (username, password_hash, email, role) VALUES (?, ?, ?, ?)",
		username, string(passwordHash), email, role,
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
		Role:     role,
	}, nil
}

// GetUserByUsername retrieves a user by username
func (s *Storage) GetUserByUsername(username string) (*User, error) {
	var user User
	err := s.db.QueryRow(
		"SELECT id, username, password_hash, role, email, created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Email, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// ValidatePassword validates a user's password against a bcrypt hash.
func (s *Storage) ValidatePassword(username, password string) (*User, error) {
	user, err := s.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}

	if !isBcryptHash(user.PasswordHash) {
		return nil, fmt.Errorf("unsupported password hash format")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid password")
	}
	return user, nil
}

func isBcryptHash(hash string) bool {
	return strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$")
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// StoreRefreshToken stores a refresh token hash.
func (s *Storage) StoreRefreshToken(username, token string, expiresAt time.Time) error {
	tokenHash := hashToken(token)
	_, err := s.db.Exec(
		"INSERT INTO refresh_tokens (token_hash, username, expires_at) VALUES (?, ?, ?)",
		tokenHash, username, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to store refresh token: %w", err)
	}
	return nil
}

// ConsumeRefreshToken validates and revokes a refresh token, returning the username.
func (s *Storage) ConsumeRefreshToken(token string) (string, error) {
	tokenHash := hashToken(token)

	var username string
	var expiresAt time.Time
	var revokedAt sql.NullTime

	err := s.db.QueryRow(
		"SELECT username, expires_at, revoked_at FROM refresh_tokens WHERE token_hash = ?",
		tokenHash,
	).Scan(&username, &expiresAt, &revokedAt)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("refresh token not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to read refresh token: %w", err)
	}

	if revokedAt.Valid {
		return "", fmt.Errorf("refresh token revoked")
	}
	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("refresh token expired")
	}

	if _, err := s.db.Exec(
		"UPDATE refresh_tokens SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash = ?",
		tokenHash,
	); err != nil {
		return "", fmt.Errorf("failed to revoke refresh token: %w", err)
	}

	return username, nil
}

// RevokeRefreshToken revokes a refresh token if present.
func (s *Storage) RevokeRefreshToken(token string) error {
	tokenHash := hashToken(token)
	if _, err := s.db.Exec(
		"UPDATE refresh_tokens SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash = ?",
		tokenHash,
	); err != nil {
		return fmt.Errorf("failed to revoke refresh token: %w", err)
	}
	return nil
}

// RevokeToken revokes an access token by jti.
func (s *Storage) RevokeToken(jti string) error {
	if jti == "" {
		return fmt.Errorf("token id is required")
	}
	if _, err := s.db.Exec(
		"INSERT OR IGNORE INTO revoked_tokens (jti, revoked_at) VALUES (?, CURRENT_TIMESTAMP)",
		jti,
	); err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	return nil
}

// IsTokenRevoked checks if an access token id is revoked.
func (s *Storage) IsTokenRevoked(jti string) (bool, error) {
	var exists int
	err := s.db.QueryRow("SELECT COUNT(*) FROM revoked_tokens WHERE jti = ?", jti).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check token revocation: %w", err)
	}
	return exists > 0, nil
}

// SeedPolicies creates an initial policy version if none exists.
func (s *Storage) SeedPolicies(cfg config.PoliciesConfig) error {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM policy_versions").Scan(&count); err != nil {
		return fmt.Errorf("failed to count policy versions: %w", err)
	}
	if count > 0 {
		return nil
	}

	versionID, err := s.createPolicyVersion("seed", cfg.DefaultDeny, true)
	if err != nil {
		return err
	}

	for _, rule := range cfg.Rules {
		for _, resource := range rule.Resources {
			if _, err := s.CreatePolicyRule(versionID, PolicyRuleInput{
				SubjectType: "user",
				Subject:     rule.User,
				Resource:    resource,
				Allowed:     rule.Allowed,
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

// CreatePolicyVersion creates a new policy version.
func (s *Storage) CreatePolicyVersion(description string, defaultDeny bool) (int64, error) {
	return s.createPolicyVersion(description, defaultDeny, false)
}

func (s *Storage) createPolicyVersion(description string, defaultDeny bool, active bool) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO policy_versions (description, default_deny, active) VALUES (?, ?, ?)",
		description, defaultDeny, active,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create policy version: %w", err)
	}
	versionID, _ := result.LastInsertId()
	if active {
		if err := s.ActivatePolicyVersion(versionID); err != nil {
			return 0, err
		}
	}
	return versionID, nil
}

// ListPolicyVersions lists policy versions.
func (s *Storage) ListPolicyVersions() ([]PolicyVersion, error) {
	rows, err := s.db.Query(
		"SELECT id, description, default_deny, active, created_at FROM policy_versions ORDER BY id DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list policy versions: %w", err)
	}
	defer rows.Close()

	var versions []PolicyVersion
	for rows.Next() {
		var v PolicyVersion
		if err := rows.Scan(&v.ID, &v.Description, &v.DefaultDeny, &v.Active, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan policy version: %w", err)
		}
		versions = append(versions, v)
	}

	return versions, nil
}

// ActivatePolicyVersion marks a version as active.
func (s *Storage) ActivatePolicyVersion(versionID int64) error {
	if _, err := s.db.Exec("UPDATE policy_versions SET active = 0 WHERE active = 1"); err != nil {
		return fmt.Errorf("failed to deactivate policy versions: %w", err)
	}

	if _, err := s.db.Exec(
		"UPDATE policy_versions SET active = 1 WHERE id = ?",
		versionID,
	); err != nil {
		return fmt.Errorf("failed to activate policy version: %w", err)
	}

	return nil
}

// GetActivePolicyVersion returns the active policy version.
func (s *Storage) GetActivePolicyVersion() (*PolicyVersion, error) {
	var v PolicyVersion
	err := s.db.QueryRow(
		"SELECT id, description, default_deny, active, created_at FROM policy_versions WHERE active = 1 LIMIT 1",
	).Scan(&v.ID, &v.Description, &v.DefaultDeny, &v.Active, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active policy version")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active policy version: %w", err)
	}
	return &v, nil
}

// ListPolicyRules lists rules for a version.
func (s *Storage) ListPolicyRules(versionID int64) ([]PolicyRule, error) {
	rows, err := s.db.Query(
		"SELECT id, version_id, subject_type, subject, resource, allowed, created_at FROM policy_rules WHERE version_id = ? ORDER BY id",
		versionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list policy rules: %w", err)
	}
	defer rows.Close()

	var rules []PolicyRule
	for rows.Next() {
		var r PolicyRule
		if err := rows.Scan(&r.ID, &r.VersionID, &r.SubjectType, &r.Subject, &r.Resource, &r.Allowed, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan policy rule: %w", err)
		}
		rules = append(rules, r)
	}

	return rules, nil
}

// CreatePolicyRule creates a policy rule in a version.
func (s *Storage) CreatePolicyRule(versionID int64, input PolicyRuleInput) (PolicyRule, error) {
	result, err := s.db.Exec(
		"INSERT INTO policy_rules (version_id, subject_type, subject, resource, allowed) VALUES (?, ?, ?, ?, ?)",
		versionID, input.SubjectType, input.Subject, input.Resource, input.Allowed,
	)
	if err != nil {
		return PolicyRule{}, fmt.Errorf("failed to create policy rule: %w", err)
	}
	id, _ := result.LastInsertId()
	return PolicyRule{
		ID:          id,
		VersionID:   versionID,
		SubjectType: input.SubjectType,
		Subject:     input.Subject,
		Resource:    input.Resource,
		Allowed:     input.Allowed,
	}, nil
}

// UpdatePolicyRule updates an existing policy rule.
func (s *Storage) UpdatePolicyRule(ruleID int64, input PolicyRuleInput) error {
	if _, err := s.db.Exec(
		"UPDATE policy_rules SET subject_type = ?, subject = ?, resource = ?, allowed = ? WHERE id = ?",
		input.SubjectType, input.Subject, input.Resource, input.Allowed, ruleID,
	); err != nil {
		return fmt.Errorf("failed to update policy rule: %w", err)
	}
	return nil
}

// DeletePolicyRule deletes a policy rule.
func (s *Storage) DeletePolicyRule(ruleID int64) error {
	if _, err := s.db.Exec("DELETE FROM policy_rules WHERE id = ?", ruleID); err != nil {
		return fmt.Errorf("failed to delete policy rule: %w", err)
	}
	return nil
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
