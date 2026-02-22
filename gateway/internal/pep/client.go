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
	http     *http.Client
}

// New creates a PEP client.
func New(cpURL, pepID, pepToken string, httpClient *http.Client) *Client {
	return &Client{cpURL: cpURL, pepID: pepID, pepToken: pepToken, http: httpClient}
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

// AuthorizeRequest is the body sent to /api/v1/pep/authorize.
type AuthorizeRequest struct {
	Subject  SubjectDTO  `json:"subject"`
	Action   string      `json:"action"`
	Resource ResourceDTO `json:"resource"`
}

// AuthorizeResponse is the response from /api/v1/pep/authorize.
type AuthorizeResponse struct {
	DecisionID string `json:"decision_id"`
	Effect     string `json:"effect"`
	Reason     string `json:"reason"`
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
	httpReq.Header.Set("X-PEP-ID", c.pepID)
	httpReq.Header.Set("X-PEP-TOKEN", c.pepToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return AuthorizeResponse{}, fmt.Errorf("authorize request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return AuthorizeResponse{}, fmt.Errorf("authorize: HTTP %d: %s", resp.StatusCode, errBody.Error)
	}

	var decision AuthorizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&decision); err != nil {
		return AuthorizeResponse{}, fmt.Errorf("decode authorize response: %w", err)
	}
	return decision, nil
}

// Heartbeat sends a heartbeat to the CP.
func (c *Client) Heartbeat(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cpURL+"/api/v1/pep/heartbeat", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-PEP-ID", c.pepID)
	req.Header.Set("X-PEP-TOKEN", c.pepToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
