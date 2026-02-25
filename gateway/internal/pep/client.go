// Package pep implements the ZTNA CP PEP client.
package pep

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client calls the Control Plane PEP endpoint.
type Client struct {
	cpURL    string
	pepID    string
	pepToken string
	authMode string
	http     *http.Client
}

// New creates a PEP client.
func New(cpURL, pepID, pepToken, authMode string, httpClient *http.Client) *Client {
	return &Client{
		cpURL:    cpURL,
		pepID:    pepID,
		pepToken: pepToken,
		authMode: authMode,
		http:     httpClient,
	}
}

// SubjectDTO mirrors control-plane/internal/domain/model.Subject.
type SubjectDTO struct {
	Sub      string   `json:"sub"`
	Username string   `json:"username"`
	Email    string   `json:"email,omitempty"`
	Groups   []string `json:"groups"`
}

// SSHResource mirrors control-plane/internal/domain/model.SSHResource.
type SSHResource struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// HTTPResource is an HTTP resource.
type HTTPResource struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// ResourceDTO mirrors control-plane/internal/domain/model.Resource.
type ResourceDTO struct {
	Type string        `json:"type"`
	SSH  *SSHResource  `json:"ssh,omitempty"`
	HTTP *HTTPResource `json:"http,omitempty"`
}

// AuthorizeContext carries additional telemetry for PDP decisions.
type AuthorizeContext struct {
	SrcIP       string `json:"src_ip,omitempty"`
	GatewayID   string `json:"gateway_id,omitempty"`
	SessionHint string `json:"session_hint,omitempty"`
}

// AuthorizeRequest is the body sent to /api/v1/pep/authorize.
type AuthorizeRequest struct {
	Subject  SubjectDTO       `json:"subject"`
	Action   string           `json:"action"`
	Resource ResourceDTO      `json:"resource"`
	Context  AuthorizeContext `json:"context,omitempty"`
}

// AuthorizeResponse is the response from /api/v1/pep/authorize.
type AuthorizeResponse struct {
	DecisionID    string `json:"decision_id"`
	Effect        string `json:"effect"`
	Reason        string `json:"reason"`
	TTLSeconds    int    `json:"ttl_seconds"`
	PolicyVersion int64  `json:"policy_version"`
}

// APIError contains a structured HTTP error from the CP.
type APIError struct {
	Path       string
	StatusCode int
	Status     string
	Message    string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Status
	}
	return fmt.Sprintf("%s: HTTP %d: %s", e.Path, e.StatusCode, msg)
}

func (e *APIError) IsStatus(code int) bool {
	return e != nil && e.StatusCode == code
}

// Authorize sends an authorization request to the CP and returns the decision.
func (c *Client) Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	body, err := json.Marshal(req)
	if err != nil {
		return AuthorizeResponse{}, fmt.Errorf("marshal authorize request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cpURL+"/api/v1/pep/authorize", bytes.NewReader(body))
	if err != nil {
		return AuthorizeResponse{}, fmt.Errorf("build authorize request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.applyAuth(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return AuthorizeResponse{}, fmt.Errorf("authorize request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return AuthorizeResponse{}, decodeAPIError("/api/v1/pep/authorize", resp)
	}

	var decision AuthorizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&decision); err != nil {
		return AuthorizeResponse{}, fmt.Errorf("decode authorize response: %w", err)
	}
	return decision, nil
}

// Heartbeat sends a heartbeat to the CP.
func (c *Client) Heartbeat(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cpURL+"/api/v1/pep/heartbeat", nil)
	if err != nil {
		return "", err
	}
	c.applyAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", decodeAPIError("/api/v1/pep/heartbeat", resp)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode heartbeat response: %w", err)
	}
	if body.Status == "" {
		body.Status = "registered"
	}
	return body.Status, nil
}

// RegisterRequest is the payload sent to /api/v1/pep/register.
type RegisterRequest struct {
	GatewayID   string `json:"gateway_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Register announces this gateway to the CP inventory.
func (c *Client) Register(ctx context.Context, req RegisterRequest) error {
	return c.postPEP(ctx, "/api/v1/pep/register", req)
}

// ── Session telemetry ─────────────────────────────────────────────────────────

// SessionStartRequest est le payload envoyé à /api/v1/pep/sessions/start.
type SessionStartRequest struct {
	SessionID       string `json:"session_id"`
	DecisionID      string `json:"decision_id"`
	SubjectSub      string `json:"subject_sub"`
	SubjectUsername string `json:"subject_username"`
	DeviceSerial    string `json:"device_serial"`
	ResourceType    string `json:"resource_type"`
	ResourceMatch   string `json:"resource_match"`
}

// SessionEndRequest est le payload envoyé à /api/v1/pep/sessions/end.
type SessionEndRequest struct {
	SessionID  string `json:"session_id"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
	DurationMs int64  `json:"duration_ms"`
	EndReason  string `json:"end_reason"`
}

// SessionStart notifie le CP de l'ouverture d'une session relayée.
// Erreur non-fatale : si le CP est inaccessible, la session continue.
func (c *Client) SessionStart(ctx context.Context, req SessionStartRequest) error {
	return c.postPEP(ctx, "/api/v1/pep/sessions/start", req)
}

// SessionEnd notifie le CP de la fermeture d'une session avec sa télémétrie.
// Erreur non-fatale : si le CP est inaccessible, la session est déjà terminée.
func (c *Client) SessionEnd(ctx context.Context, req SessionEndRequest) error {
	if req.SessionID == "" {
		return nil // session non démarrée (dial_error avant SessionStart)
	}
	return c.postPEP(ctx, "/api/v1/pep/sessions/end", req)
}

// postPEP est un helper interne pour les appels POST vers les endpoints PEP.
func (c *Client) postPEP(ctx context.Context, path string, payload any) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cpURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", path, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.applyAuth(httpReq)

	// Keep transport errors explicit so callers can decide whether they are fatal.
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return decodeAPIError(path, resp)
	}
	return nil
}

func (c *Client) applyAuth(req *http.Request) {
	if c.authMode == "token" {
		req.Header.Set("X-PEP-ID", c.pepID)
		req.Header.Set("X-PEP-TOKEN", c.pepToken)
	}
}

func decodeAPIError(path string, resp *http.Response) error {
	var errBody struct {
		Error  string `json:"error"`
		Status string `json:"status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&errBody)
	return &APIError{
		Path:       path,
		StatusCode: resp.StatusCode,
		Status:     errBody.Status,
		Message:    errBody.Error,
	}
}
