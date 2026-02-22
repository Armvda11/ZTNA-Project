// Package logger fournit un logger structuré basé sur log/slog pour la
// Gateway ZTNA. Suit les mêmes conventions que le Control Plane et le Client.
package logger

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey string

const (
	keyRequestID ctxKey = "request_id"
	keySessionID ctxKey = "session_id"
	keySubject   ctxKey = "subject"
	keyPepID     ctxKey = "pep_id"
)

// New crée un logger slog configuré avec le niveau et le format indiqués.
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

// WithSessionID ajoute un identifiant de session au contexte.
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keySessionID, id)
}

// SessionIDFromContext extrait l'identifiant de session du contexte.
func SessionIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(keySessionID).(string); ok {
		return id
	}
	return ""
}

// WithSubject ajoute l'identifiant du sujet au contexte.
func WithSubject(ctx context.Context, sub string) context.Context {
	return context.WithValue(ctx, keySubject, sub)
}

// SubjectFromContext extrait l'identifiant du sujet du contexte.
func SubjectFromContext(ctx context.Context) string {
	if s, ok := ctx.Value(keySubject).(string); ok {
		return s
	}
	return ""
}
