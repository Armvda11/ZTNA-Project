package model

import "fmt"

type ResourceType string

const (
	ResourceSSH ResourceType = "ssh"
)

type Resource struct {
	Type ResourceType `json:"type"`
	SSH  *SSHResource `json:"ssh,omitempty"`
}

type SSHResource struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (r Resource) Canonical() string {
	switch r.Type {
	case ResourceSSH:
		if r.SSH == nil {
			return "ssh:unknown"
		}
		return fmt.Sprintf("ssh:%s:%d", r.SSH.Host, r.SSH.Port)
	default:
		return "unknown"
	}
}
