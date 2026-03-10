package model

import "fmt"

// ResourceType identifie le protocole de la ressource cible.
// Les types supportés sont indexés dans les politiques d'accès.
type ResourceType string

const (
	ResourceSSH  ResourceType = "ssh"  // Accès terminal SSH
	ResourceHTTP ResourceType = "http" // Accès applicatif HTTP (proxy TCP brut)
	ResourceWeb  ResourceType = "web"  // Accès web via reverse proxy HTTP
	ResourceDB   ResourceType = "db"   // Accès base de données via tunnel TCP
)

// Resource représente la ressource à laquelle un sujet tente d'accéder.
// Un seul des champs SSH / HTTP est non-nil selon le Type.
type Resource struct {
	Type ResourceType  `json:"type"`
	SSH  *SSHResource  `json:"ssh,omitempty"`
	HTTP *HTTPResource `json:"http,omitempty"`
}

// SSHResource décrit une ressource SSH (host:port).
type SSHResource struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// HTTPResource décrit une ressource HTTP (host:port).
// Le gateway proxifie le trafic TCP brut ; le protocole HTTP lui-même
// n'est pas inspecté (pas de terminaison HTTP côté gateway).
type HTTPResource struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Canonical retourne la forme canonique lisible par les politiques,
// utilisée dans le moteur d'évaluation et dans le journal d'audit.
// Format : "<type>:<host>:<port>"  ex: "ssh:lan-app:22", "http:lan-app:80"
func (r Resource) Canonical() string {
	switch r.Type {
	case ResourceSSH:
		if r.SSH == nil {
			return "ssh:unknown"
		}
		return fmt.Sprintf("ssh:%s:%d", r.SSH.Host, r.SSH.Port)
	case ResourceHTTP:
		if r.HTTP == nil {
			return "http:unknown"
		}
		return fmt.Sprintf("http:%s:%d", r.HTTP.Host, r.HTTP.Port)
	default:
		return "unknown"
	}
}
