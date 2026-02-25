package decision

import (
	"context"

	"github.com/google/uuid"

	"control-plane/internal/domain/model"
	"control-plane/internal/service/policy"
)

type Service struct {
	policy     *policy.Service
	ttlSeconds int
}

// New creates the decision service. ttlSeconds is the advertised cache TTL
// returned to the gateway for each decision (default: 60).
func New(policySvc *policy.Service, ttlSeconds int) *Service {
	if ttlSeconds <= 0 {
		ttlSeconds = 60
	}
	return &Service{policy: policySvc, ttlSeconds: ttlSeconds}
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
	// Use UUID v4 to guarantee global uniqueness even under concurrent load.
	decisionID := "dec-" + uuid.New().String()

	return model.Decision{
		Effect:        effect,
		Reason:        reason,
		TTLSeconds:    s.ttlSeconds,
		PolicyVersion: snapshot.Version.ID,
		DecisionID:    decisionID,
	}, nil
}
