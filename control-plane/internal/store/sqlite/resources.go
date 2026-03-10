package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"control-plane/internal/domain/model"
)

func (s *Store) CreateResource(ctx context.Context, res model.PublishedResource) error {
	groupsJSON, err := json.Marshal(res.GroupMatch)
	if err != nil {
		return fmt.Errorf("marshal group_match: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO resources(id, name, display_name, type, backend, gateway_id, group_match, access_mode, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		res.ID, res.Name, res.DisplayName, res.Type, res.Backend,
		res.GatewayID, string(groupsJSON), string(res.AccessMode),
		res.Description, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert resource: %w", err)
	}
	return nil
}

func (s *Store) UpdateResource(ctx context.Context, res model.PublishedResource) error {
	groupsJSON, err := json.Marshal(res.GroupMatch)
	if err != nil {
		return fmt.Errorf("marshal group_match: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`UPDATE resources SET display_name=?, type=?, backend=?, gateway_id=?, group_match=?, access_mode=?, description=?, updated_at=?
		 WHERE name=?`,
		res.DisplayName, res.Type, res.Backend,
		res.GatewayID, string(groupsJSON), string(res.AccessMode),
		res.Description, now, res.Name,
	)
	if err != nil {
		return fmt.Errorf("update resource: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetResourceByName(ctx context.Context, name string) (model.PublishedResource, error) {
	var res model.PublishedResource
	var groupsJSON string
	var accessMode string
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, display_name, type, backend, gateway_id, group_match, access_mode, description, created_at, updated_at
		 FROM resources WHERE name = ?`, name,
	).Scan(&res.ID, &res.Name, &res.DisplayName, &res.Type, &res.Backend,
		&res.GatewayID, &groupsJSON, &accessMode,
		&res.Description, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return res, err
		}
		return res, fmt.Errorf("get resource by name: %w", err)
	}
	res.AccessMode = model.AccessMode(accessMode)
	_ = json.Unmarshal([]byte(groupsJSON), &res.GroupMatch)
	res.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	res.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return res, nil
}

func (s *Store) ListResources(ctx context.Context) ([]model.PublishedResource, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, display_name, type, backend, gateway_id, group_match, access_mode, description, created_at, updated_at
		 FROM resources ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}
	defer rows.Close()
	return scanResources(rows)
}

func (s *Store) ListResourcesForGroups(ctx context.Context, groups []string) ([]model.PublishedResource, error) {
	// Fetch all resources and filter in Go — SQLite JSON support is limited.
	all, err := s.ListResources(ctx)
	if err != nil {
		return nil, err
	}
	groupSet := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		groupSet[g] = struct{}{}
	}
	var result []model.PublishedResource
	for _, res := range all {
		if len(res.GroupMatch) == 0 {
			// No group restriction → visible to all authenticated users.
			result = append(result, res)
			continue
		}
		for _, g := range res.GroupMatch {
			if _, ok := groupSet[g]; ok {
				result = append(result, res)
				break
			}
		}
	}
	return result, nil
}

func (s *Store) DeleteResource(ctx context.Context, name string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM resources WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanResources(rows *sql.Rows) ([]model.PublishedResource, error) {
	var resources []model.PublishedResource
	for rows.Next() {
		var res model.PublishedResource
		var groupsJSON, accessMode, createdAt, updatedAt string
		if err := rows.Scan(&res.ID, &res.Name, &res.DisplayName, &res.Type, &res.Backend,
			&res.GatewayID, &groupsJSON, &accessMode,
			&res.Description, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan resource: %w", err)
		}
		res.AccessMode = model.AccessMode(accessMode)
		_ = json.Unmarshal([]byte(groupsJSON), &res.GroupMatch)
		res.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		res.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		resources = append(resources, res)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read resources: %w", err)
	}
	return resources, nil
}
