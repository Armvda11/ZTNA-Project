// Package handlers contient des helpers et handlers HTTP utilisés
// par l'API du control-plane.
package handlers

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"control-plane/internal/domain/model"
)

func extractContextIP(ctx map[string]any) string {
	if ctx == nil {
		return ""
	}
	raw, ok := ctx["src_ip"]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if net.ParseIP(value) == nil {
		return ""
	}
	return value
}

// extractRemoteIP extrait l'adresse IP cliente à partir des entêtes
// X-Forwarded-For, X-Real-IP ou du RemoteAddr. Retourne une chaîne
// vide si aucune IP valide n'est trouvée.

func extractRemoteIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if net.ParseIP(realIP) != nil {
			return realIP
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if net.ParseIP(host) != nil {
			return host
		}
	}
	if net.ParseIP(r.RemoteAddr) != nil {
		return r.RemoteAddr
	}
	return ""
}

func formatSubjectForAudit(subject model.Subject) string {
	if subject.Username == "" && subject.Sub == "" {
		return ""
	}
	if subject.Username == "" {
		return subject.Sub
	}
	if subject.Sub == "" {
		return subject.Username
	}
	return fmt.Sprintf("%s|%s", subject.Username, subject.Sub)
}
