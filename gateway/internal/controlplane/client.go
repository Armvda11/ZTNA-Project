package controlplane

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ztna/gateway/internal/config"
	"github.com/ztna/gateway/internal/logger"
)

// Client represents a Control Plane API client
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *logger.Logger
}

// NewClient creates a new Control Plane client
func NewClient(cfg config.ControlPlaneConfig, log *logger.Logger) *Client {
	// Create HTTP client with optional TLS skip verification
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.TLSSkipVerify,
		},
	}

	return &Client{
		baseURL: cfg.URL,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
		logger: log,
	}
}

// GetCAPublicKey retrieves the CA public key from Control Plane
func (c *Client) GetCAPublicKey(endpoint string) (string, error) {
	url := c.baseURL + endpoint

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get CA public key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("CA public key request failed: %d - %s", resp.StatusCode, string(body))
	}

	var result struct {
		PublicKey string `json:"public_key"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse CA public key response: %w", err)
	}

	return result.PublicKey, nil
}

// PolicyCheckRequest represents a policy check request
type PolicyCheckRequest struct {
	Username string `json:"username"`
	Resource string `json:"resource"`
}

// PolicyCheckResponse represents a policy check response
type PolicyCheckResponse struct {
	Allowed bool   `json:"allowed"`
	Message string `json:"message,omitempty"`
}

// CheckPolicy checks if a user can access a resource
func (c *Client) CheckPolicy(endpoint, username, resource, token string) (*PolicyCheckResponse, error) {
	url := fmt.Sprintf("%s%s/%s", c.baseURL, endpoint, resource)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create policy check request: %w", err)
	}

	// Add JWT token if provided
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check policy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("policy check failed: %d - %s", resp.StatusCode, string(body))
	}

	var result PolicyCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse policy check response: %w", err)
	}

	return &result, nil
}

// HealthCheck checks if the Control Plane is reachable
func (c *Client) HealthCheck() error {
	url := c.baseURL + "/health"

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status: %d", resp.StatusCode)
	}

	return nil
}
