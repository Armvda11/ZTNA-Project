package audit

import (
	"context"
	"time"

	"control-plane/internal/domain/model"
	"control-plane/internal/domain/port"
)

type Service struct {
	repo port.AuditRepository
}

func New(repo port.AuditRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Append(ctx context.Context, event model.AuditEvent) error {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	return s.repo.InsertAuditEvent(ctx, event)
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]model.AuditEvent, error) {
	return s.repo.ListAuditEvents(ctx, limit, offset)
}
