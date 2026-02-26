package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"control-plane/internal/domain/model"
)

// CreateSession persiste une nouvelle session (état "active", end_time NULL).
func (s *Store) CreateSession(ctx context.Context, sess model.Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions
		  (session_id, decision_id, subject_sub, subject_username,
		   device_serial, resource_type, resource_match, start_time)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.SessionID,
		sess.DecisionID,
		sess.SubjectSub,
		sess.SubjectUsername,
		sess.DeviceSerial,
		sess.ResourceType,
		sess.ResourceMatch,
		sess.StartTime,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// CompleteSession met à jour une session existante avec les métriques de fin.
func (s *Store) CompleteSession(ctx context.Context, sess model.Session) error {
	endTime := sess.EndTime
	if endTime == "" {
		endTime = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions
		    SET end_time    = ?,
		        bytes_in    = ?,
		        bytes_out   = ?,
		        duration_ms = ?,
		        end_reason  = ?
		  WHERE session_id  = ?`,
		endTime,
		sess.BytesIn,
		sess.BytesOut,
		sess.DurationMs,
		sess.EndReason,
		sess.SessionID,
	)
	if err != nil {
		return fmt.Errorf("complete session: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("complete session: not found %s", sess.SessionID)
	}
	return nil
}

// ListSessions retourne les sessions triées par start_time desc, avec une limit.
func (s *Store) ListSessions(ctx context.Context, limit int) ([]model.Session, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, decision_id, subject_sub, subject_username,
		        device_serial, resource_type, resource_match,
		        start_time, COALESCE(end_time,''), bytes_in, bytes_out,
		        duration_ms, COALESCE(end_reason,''),
		        COALESCE(killed_at,''), COALESCE(killed_by,'')
		   FROM sessions
		  ORDER BY start_time DESC
		  LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var sess model.Session
		if err := rows.Scan(
			&sess.SessionID, &sess.DecisionID,
			&sess.SubjectSub, &sess.SubjectUsername,
			&sess.DeviceSerial, &sess.ResourceType, &sess.ResourceMatch,
			&sess.StartTime, &sess.EndTime,
			&sess.BytesIn, &sess.BytesOut,
			&sess.DurationMs, &sess.EndReason,
			&sess.KilledAt, &sess.KilledBy,
		); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// GetSession retourne une session par son ID.
func (s *Store) GetSession(ctx context.Context, sessionID string) (model.Session, error) {
	var sess model.Session
	err := s.db.QueryRowContext(ctx,
		`SELECT session_id, decision_id, subject_sub, subject_username,
		        device_serial, resource_type, resource_match,
		        start_time, COALESCE(end_time,''), bytes_in, bytes_out,
		        duration_ms, COALESCE(end_reason,''),
		        COALESCE(killed_at,''), COALESCE(killed_by,'')
		   FROM sessions WHERE session_id = ?`, sessionID).Scan(
		&sess.SessionID, &sess.DecisionID,
		&sess.SubjectSub, &sess.SubjectUsername,
		&sess.DeviceSerial, &sess.ResourceType, &sess.ResourceMatch,
		&sess.StartTime, &sess.EndTime,
		&sess.BytesIn, &sess.BytesOut,
		&sess.DurationMs, &sess.EndReason,
		&sess.KilledAt, &sess.KilledBy,
	)
	if err != nil {
		return model.Session{}, fmt.Errorf("get session %s: %w", sessionID, err)
	}
	return sess, nil
}

// KillSession marque une session active comme tuée par un admin.
// Retourne false si la session n'existe pas.
func (s *Store) KillSession(ctx context.Context, sessionID string, killedBy string) (bool, error) {
	killedAt := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET killed_at = ?, killed_by = ?
		  WHERE session_id = ? AND killed_at IS NULL`,
		killedAt, killedBy, sessionID,
	)
	if err != nil {
		return false, fmt.Errorf("kill session %s: %w", sessionID, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// IsSessionValid retourne true si la session existe et n'a pas été tuée.
// Retourne true aussi si la session est introuvable (session pas encore enregistrée
// ou déjà purgée) pour ne pas couper par erreur.
func (s *Store) IsSessionValid(ctx context.Context, sessionID string) (bool, error) {
	var killedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(killed_at,'') FROM sessions WHERE session_id = ?`, sessionID).Scan(&killedAt)
	if err == sql.ErrNoRows {
		return true, nil // session inconnue → fail-open
	}
	if err != nil {
		return true, fmt.Errorf("is session valid %s: %w", sessionID, err)
	}
	return killedAt == "", nil
}
