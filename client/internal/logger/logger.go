// Package logger fournit un logger structuré basé sur log/slog pour le
// client ZTNA. Il suit les mêmes conventions que le Control Plane :
// factory New(level, format) et propagation de contexte.
package logger

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey string

const (
	keyRequestID ctxKey = "request_id"
	keyResource  ctxKey = "resource"
)

// New crée un logger slog configuré avec le niveau et le format indiqués.
// Formats supportés : "text" pour un affichage humain, "json" (défaut)
// pour un format structuré machine-readable.
func New(level string, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// WithRequestID ajoute un identifiant de requête au contexte.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestIDFromContext extrait l'identifiant de requête du contexte.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(keyRequestID).(string); ok {
		return id
	}
	return ""
}

// WithResource ajoute le nom de la ressource ciblée au contexte.
func WithResource(ctx context.Context, resource string) context.Context {
	return context.WithValue(ctx, keyResource, resource)
}

// ResourceFromContext extrait le nom de la ressource du contexte.
func ResourceFromContext(ctx context.Context) string {
	if r, ok := ctx.Value(keyResource).(string); ok {
		return r
	}
	return ""
}
