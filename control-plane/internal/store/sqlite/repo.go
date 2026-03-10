package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"control-plane/internal/domain/model"
)

func (s *Store) UpsertUser(ctx context.Context, subject model.Subject) error {
	groups, err := json.Marshal(subject.Groups)
	if err != nil {
		return fmt.Errorf("marshal groups: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO users(sub, username, groups_json, created_at)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(sub) DO UPDATE SET username = excluded.username, groups_json = excluded.groups_json`,
		subject.Sub, subject.Username, string(groups), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	return nil
}

func (s *Store) CreatePolicyVersion(ctx context.Context, createdBy string, rules []model.PolicyRule) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin create policy: %w", err)
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO policy_versions(active, created_at, created_by)
        VALUES (0, ?, ?)`, time.Now().UTC().Format(time.RFC3339), createdBy)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("insert policy version: %w", err)
	}

	versionID, err := res.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("policy version id: %w", err)
	}

	for _, rule := range rules {
		if _, err := tx.ExecContext(ctx, `INSERT INTO policy_rules(version_id, effect, subject_match, action, resource_type, resource_match, allowed_hours, required_device_trust, created_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			versionID, rule.Effect, rule.SubjectMatch, rule.Action, rule.ResourceType, rule.ResourceMatch, rule.AllowedHours, rule.RequiredDeviceTrust, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("insert policy rule: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit policy version: %w", err)
	}

	return versionID, nil
}

func (s *Store) ActivatePolicyVersion(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin activate policy: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "UPDATE policy_versions SET active = 0"); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("deactivate policies: %w", err)
	}

	res, err := tx.ExecContext(ctx, "UPDATE policy_versions SET active = 1 WHERE id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("activate policy version: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		_ = tx.Rollback()
		return sql.ErrNoRows
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit activate policy: %w", err)
	}

	return nil
}

func (s *Store) GetActivePolicy(ctx context.Context) (model.PolicySnapshot, error) {
	var snapshot model.PolicySnapshot

	row := s.db.QueryRowContext(ctx, `SELECT id, active, created_at, created_by
        FROM policy_versions WHERE active = 1 ORDER BY created_at DESC LIMIT 1`)

	if err := row.Scan(&snapshot.Version.ID, &snapshot.Version.Active, &snapshot.Version.CreatedAt, &snapshot.Version.CreatedBy); err != nil {
		if err == sql.ErrNoRows {
			return snapshot, err
		}
		return snapshot, fmt.Errorf("scan active policy: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, version_id, effect, subject_match, action, resource_type, resource_match, allowed_hours, required_device_trust, created_at
        FROM policy_rules WHERE version_id = ? ORDER BY id ASC`, snapshot.Version.ID)
	if err != nil {
		return snapshot, fmt.Errorf("list policy rules: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var rule model.PolicyRule
		if err := rows.Scan(&rule.ID, &rule.VersionID, &rule.Effect, &rule.SubjectMatch, &rule.Action, &rule.ResourceType, &rule.ResourceMatch, &rule.AllowedHours, &rule.RequiredDeviceTrust, &rule.CreatedAt); err != nil {
			return snapshot, fmt.Errorf("scan policy rule: %w", err)
		}
		snapshot.Rules = append(snapshot.Rules, rule)
	}

	if err := rows.Err(); err != nil {
		return snapshot, fmt.Errorf("read policy rules: %w", err)
	}

	return snapshot, nil
}

func (s *Store) InsertAuditEvent(ctx context.Context, event model.AuditEvent) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_events(ts, subject, action, resource, decision, reason, pep_id, src_ip, policy_version)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.Timestamp, event.Subject, event.Action, event.Resource, event.Decision, event.Reason, event.PepID, event.SourceIP, event.PolicyVersion)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func (s *Store) ListAuditEvents(ctx context.Context, limit, offset int) ([]model.AuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, subject, action, resource, decision, reason, pep_id, src_ip, policy_version
        FROM audit_events ORDER BY ts DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	var events []model.AuditEvent
	for rows.Next() {
		var event model.AuditEvent
		if err := rows.Scan(&event.ID, &event.Timestamp, &event.Subject, &event.Action, &event.Resource, &event.Decision, &event.Reason, &event.PepID, &event.SourceIP, &event.PolicyVersion); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read audit events: %w", err)
	}

	return events, nil
}
