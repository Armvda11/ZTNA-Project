// Package lifecycle contient les use-cases transverses de cycle de vie.
package lifecycle

import "context"

// GracefulShutdown est un hook d'orchestration pour arrêt propre.
// TODO: brancher fermeture listener, drain sessions, flush métriques/audit.
func GracefulShutdown(ctx context.Context) error {
	_ = ctx
	return nil
}
