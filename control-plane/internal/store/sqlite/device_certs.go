package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"control-plane/internal/domain/model"
)

// StoreDeviceCert persists a newly issued X.509 device certificate.
func (s *Store) StoreDeviceCert(ctx context.Context, cert model.DeviceCert) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO device_certs(serial, sub, username, fingerprint, issued_at, expires_at, revoked)
         VALUES (?, ?, ?, ?, ?, ?, 0)`,
		cert.Serial, cert.Sub, cert.Username, cert.Fingerprint, cert.IssuedAt, cert.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store device cert: %w", err)
	}
	return nil
}

// RevokeDeviceCert marks an existing certificate as revoked.
func (s *Store) RevokeDeviceCert(ctx context.Context, serial, reason string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE device_certs SET revoked = 1, revoked_at = ?, revocation_reason = ?
         WHERE serial = ? AND revoked = 0`,
		time.Now().UTC().Format(time.RFC3339), reason, serial)
	if err != nil {
		return fmt.Errorf("revoke device cert: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListRevokedDeviceCerts returns all revoked certificates (used for CRL generation).
func (s *Store) ListRevokedDeviceCerts(ctx context.Context) ([]model.DeviceCert, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, serial, sub, username, fingerprint, issued_at, expires_at,
                revoked, COALESCE(revoked_at,''), COALESCE(revocation_reason,'')
         FROM device_certs WHERE revoked = 1 ORDER BY revoked_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list revoked device certs: %w", err)
	}
	defer rows.Close()

	var certs []model.DeviceCert
	for rows.Next() {
		var c model.DeviceCert
		var revoked int
		if err := rows.Scan(&c.ID, &c.Serial, &c.Sub, &c.Username, &c.Fingerprint,
			&c.IssuedAt, &c.ExpiresAt, &revoked, &c.RevokedAt, &c.RevocationReason); err != nil {
			return nil, fmt.Errorf("scan revoked device cert: %w", err)
		}
		c.Revoked = revoked == 1
		certs = append(certs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read revoked device certs: %w", err)
	}
	return certs, nil
}

// GetDeviceCert returns a device certificate by serial number.
func (s *Store) GetDeviceCert(ctx context.Context, serial string) (model.DeviceCert, error) {
	var c model.DeviceCert
	var revoked int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, serial, sub, username, fingerprint, issued_at, expires_at,
                revoked, COALESCE(revoked_at,''), COALESCE(revocation_reason,'')
         FROM device_certs WHERE serial = ?`, serial).
		Scan(&c.ID, &c.Serial, &c.Sub, &c.Username, &c.Fingerprint,
			&c.IssuedAt, &c.ExpiresAt, &revoked, &c.RevokedAt, &c.RevocationReason)
	if err == sql.ErrNoRows {
		return c, sql.ErrNoRows
	}
	if err != nil {
		return c, fmt.Errorf("get device cert: %w", err)
	}
	c.Revoked = revoked == 1
	return c, nil
}
