package sqlite

import (
	"context"
	"fmt"
	"time"

	"control-plane/internal/domain/model"
)

// RegisterGateway inserts a gateway record. If the ID already exists, the
// registration is a no-op (idempotent re-registration via ON CONFLICT DO NOTHING).
func (s *Store) RegisterGateway(ctx context.Context, gw model.Gateway) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO gateways(id, name, registered_at, active)
         VALUES (?, ?, ?, 1)
         ON CONFLICT(id) DO UPDATE SET name = excluded.name, active = 1`,
		gw.ID, gw.Name, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("register gateway: %w", err)
	}
	return nil
}

// UpdateGatewayHeartbeat records the last-seen timestamp for a gateway.
func (s *Store) UpdateGatewayHeartbeat(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE gateways SET last_seen = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("update gateway heartbeat: %w", err)
	}
	return nil
}

// ListGateways returns all registered gateways ordered by registration date.
func (s *Store) ListGateways(ctx context.Context) ([]model.Gateway, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, registered_at, COALESCE(last_seen,''), active
         FROM gateways ORDER BY registered_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list gateways: %w", err)
	}
	defer rows.Close()

	var gws []model.Gateway
	for rows.Next() {
		var gw model.Gateway
		var active int
		if err := rows.Scan(&gw.ID, &gw.Name, &gw.RegisteredAt, &gw.LastSeen, &active); err != nil {
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
