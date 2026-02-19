package audit

import (
	"context"
	"time"

	"control-plane/internal/domain/model"
	"control-plane/internal/store/sqlite"
)

type Service struct {
	store *sqlite.Store
}

func New(store *sqlite.Store) *Service {
	return &Service{store: store}
}

func (s *Service) Append(ctx context.Context, event model.AuditEvent) error {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	return s.store.InsertAuditEvent(ctx, event)
}

func (s *Service) List(ctx context.Context, limit int) ([]model.AuditEvent, error) {
	return s.store.ListAuditEvents(ctx, limit)
}
