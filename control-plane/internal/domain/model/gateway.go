package model

// Gateway represents a registered PEP (gateway) instance.
type Gateway struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	RegisteredAt    string `json:"registered_at"`
	LastSeen        string `json:"last_seen,omitempty"`
	Active          bool   `json:"active"`
	Fingerprint     string `json:"fingerprint,omitempty"`
	SoftwareVersion string `json:"software_version,omitempty"`
}
