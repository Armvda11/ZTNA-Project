// Package session fournit le service de télémétrie de sessions ZTNA.
// Il reçoit les événements session_start / session_end depuis la gateway
// et les persiste pour corrélation avec les décisions PDP (audit).
package session

import (
	"context"
	"fmt"
	"time"

	"control-plane/internal/domain/model"
)

// Store est l'interface de persistance des sessions.
type Store interface {
	CreateSession(ctx context.Context, sess model.Session) error
	CompleteSession(ctx context.Context, sess model.Session) error
	ListSessions(ctx context.Context, limit int) ([]model.Session, error)
}

// Service gère le cycle de vie des sessions TCP relayées.
type Service struct {
	store Store
}

// New crée un Service de session.
func New(store Store) *Service {
	return &Service{store: store}
}

// StartRequest est la demande d'ouverture de session reçue du PEP (gateway).
type StartRequest struct {
	SessionID       string `json:"session_id"`
	DecisionID      string `json:"decision_id"`
	SubjectSub      string `json:"subject_sub"`
	SubjectUsername string `json:"subject_username"`
	DeviceSerial    string `json:"device_serial"`
	ResourceType    string `json:"resource_type"`
	ResourceMatch   string `json:"resource_match"`
}

// EndRequest est la demande de fermeture de session reçue du PEP.
type EndRequest struct {
	SessionID  string `json:"session_id"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
	DurationMs int64  `json:"duration_ms"`
	EndReason  string `json:"end_reason"`
}

// Start crée l'enregistrement de session initial (état actif).
func (svc *Service) Start(ctx context.Context, req StartRequest) error {
	if req.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if req.DecisionID == "" {
		return fmt.Errorf("decision_id is required")
	}
	sess := model.Session{
		SessionID:       req.SessionID,
		DecisionID:      req.DecisionID,
		SubjectSub:      req.SubjectSub,
		SubjectUsername: req.SubjectUsername,
		DeviceSerial:    req.DeviceSerial,
		ResourceType:    req.ResourceType,
		ResourceMatch:   req.ResourceMatch,
		StartTime:       time.Now().UTC().Format(time.RFC3339),
	}
	return svc.store.CreateSession(ctx, sess)
}

// End complète la session avec les métriques de transfert et la raison de fin.
func (svc *Service) End(ctx context.Context, req EndRequest) error {
	if req.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	sess := model.Session{
		SessionID:  req.SessionID,
		BytesIn:    req.BytesIn,
		BytesOut:   req.BytesOut,
		DurationMs: req.DurationMs,
		EndReason:  req.EndReason,
		EndTime:    time.Now().UTC().Format(time.RFC3339),
	}
	return svc.store.CompleteSession(ctx, sess)
}

// List retourne les dernières sessions, pour l'interface d'audit admin.
func (svc *Service) List(ctx context.Context, limit int) ([]model.Session, error) {
	return svc.store.ListSessions(ctx, limit)
}
