package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
)

// RegisterGateway inserts a gateway record. If the ID already exists, the
// registration is a no-op (idempotent re-registration via ON CONFLICT DO NOTHING).
func (s *Store) RegisterGateway(ctx context.Context, gw model.Gateway) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO gateways(id, name, registered_at, active, fingerprint, software_version)
         VALUES (?, ?, ?, 1, ?, ?)
         ON CONFLICT(id) DO UPDATE SET
           name = excluded.name,
           active = 1,
           fingerprint = excluded.fingerprint,
           software_version = excluded.software_version`,
		gw.ID,
		gw.Name,
		time.Now().UTC().Format(time.RFC3339),
		gw.Fingerprint,
		gw.SoftwareVersion,
	)
	if err != nil {
		return fmt.Errorf("register gateway: %w", err)
	}
	return nil
}

// UpdateGatewayHeartbeat records the last-seen timestamp for a gateway.
func (s *Store) UpdateGatewayHeartbeat(ctx context.Context, id, version string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	query := `UPDATE gateways SET last_seen = ? WHERE id = ?`
	args := []any{now, id}
	if version != "" {
		query = `UPDATE gateways SET last_seen = ?, software_version = ? WHERE id = ?`
		args = []any{now, version, id}
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update gateway heartbeat: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domainErrors.ErrNotFound
	}
	return nil
}

// GetGateway returns one registered gateway by ID.
func (s *Store) GetGateway(ctx context.Context, id string) (model.Gateway, error) {
	var gw model.Gateway
	var active int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, registered_at, COALESCE(last_seen,''), active,
                COALESCE(fingerprint,''), COALESCE(software_version,'')
         FROM gateways WHERE id = ?`,
		id,
	).Scan(
		&gw.ID,
		&gw.Name,
		&gw.RegisteredAt,
		&gw.LastSeen,
		&active,
		&gw.Fingerprint,
		&gw.SoftwareVersion,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.Gateway{}, domainErrors.ErrNotFound
		}
		return model.Gateway{}, fmt.Errorf("get gateway: %w", err)
	}
	gw.Active = active == 1
	return gw, nil
}

// SetGatewayActive marks a gateway as active/revoked.
func (s *Store) SetGatewayActive(ctx context.Context, id string, active bool) error {
	val := 0
	if active {
		val = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE gateways SET active = ? WHERE id = ?`, val, id)
	if err != nil {
		return fmt.Errorf("set gateway active: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domainErrors.ErrNotFound
	}
	return nil
}

// ListGateways returns all registered gateways ordered by registration date.
func (s *Store) ListGateways(ctx context.Context) ([]model.Gateway, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, registered_at, COALESCE(last_seen,''), active,
                COALESCE(fingerprint,''), COALESCE(software_version,'')
         FROM gateways ORDER BY registered_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list gateways: %w", err)
	}
	defer rows.Close()

	var gws []model.Gateway
	for rows.Next() {
		var gw model.Gateway
		var active int
		if err := rows.Scan(
			&gw.ID,
			&gw.Name,
			&gw.RegisteredAt,
			&gw.LastSeen,
			&active,
			&gw.Fingerprint,
			&gw.SoftwareVersion,
		); err != nil {
			return nil, fmt.Errorf("scan gateway: %w", err)
		}
		gw.Active = active == 1
		gws = append(gws, gw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read gateways: %w", err)
	}
	return gws, nil
}
