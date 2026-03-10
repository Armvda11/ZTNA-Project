package model

import "time"

// AccessMode defines how a resource is accessed through the gateway.
type AccessMode string

const (
	AccessHTTPProxy AccessMode = "http-proxy"  // HTTP reverse proxy (web apps)
	AccessSSHCert   AccessMode = "ssh-cert"    // SSH via ephemeral cert
	AccessTCPTunnel AccessMode = "tcp-tunnel"  // Raw TCP tunnel (databases)
)

// PublishedResource is a centrally-managed resource registered in the
// control plane. A resource that doesn't exist here doesn't exist in
// the system — gateways refuse anything not explicitly published.
type PublishedResource struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`         // unique slug: "grafana-internal", "ssh-dev-01"
	DisplayName string     `json:"display_name"`  // human-readable: "Grafana Monitoring"
	Type        string     `json:"type"`          // "web", "ssh", "db"
	Backend     string     `json:"backend"`       // real endpoint: "10.10.30.15:3000"
	GatewayID   string     `json:"gateway_id"`    // which gateway serves this resource
	GroupMatch  []string   `json:"group_match"`   // groups allowed to see/list this resource
	AccessMode  AccessMode `json:"access_mode"`   // how the gateway proxies traffic
	Description string     `json:"description"`   // optional description
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
