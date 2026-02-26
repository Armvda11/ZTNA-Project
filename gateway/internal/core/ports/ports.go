// Package ports définit les interfaces cœur pour découpler use-cases et implémentations infra.
package ports

import "context"

type HealthChecker interface {
	Health(ctx context.Context) error
}

// TODO: ajouter interfaces Authorizer, ProxyEngine, SessionStore, RevocationChecker
// au fur et à mesure de l'implémentation métier.
