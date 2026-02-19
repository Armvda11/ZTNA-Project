package decision

import (
	"context"
	"fmt"
	"time"

	"control-plane/internal/domain/model"
	"control-plane/internal/service/policy"
)

type Service struct {
	policy *policy.Service
}

func New(policySvc *policy.Service) *Service {
	return &Service{policy: policySvc}
}

type AuthorizeRequest struct {
	Subject  model.Subject
	Action   string
	Resource model.Resource
	Context  map[string]any
}

func (s *Service) Authorize(ctx context.Context, req AuthorizeRequest) (model.Decision, error) {
	snapshot, err := s.policy.GetActive(ctx)
	if err != nil {
		return model.Decision{}, err
	}

	effect, reason := s.policy.Evaluate(snapshot, req.Subject, req.Action, req.Resource)
	decisionID := fmt.Sprintf("dec-%d", time.Now().UTC().UnixNano())

	return model.Decision{
		Effect:        effect,
		Reason:        reason,
		TTLSeconds:    60,
		PolicyVersion: snapshot.Version.ID,
		DecisionID:    decisionID,
	}, nil
}
