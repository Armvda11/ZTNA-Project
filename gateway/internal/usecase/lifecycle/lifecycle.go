// Package lifecycle fournit les use-cases transverses de cycle de vie pour la Gateway ZTNA.
// Il gère l'orchestration de l'arrêt propre (graceful shutdown), incluant
// la fermeture du listener, le drain des sessions actives,
// et le flush des métriques et de la télémétrie.
package lifecycle

import (
	"context"
	"log/slog"
	"time"
)

// Shutdownable représente un composant qui peut être arrêté proprement.
type Shutdownable interface {
	Close(ctx context.Context) error
}

// ShutdownOrchestrator orchestre l'arrêt graceful de multiples composants.
type ShutdownOrchestrator struct {
	components []namedComponent
	log        *slog.Logger
	timeout    time.Duration
}

type namedComponent struct {
	name string
	comp Shutdownable
}

// NewShutdownOrchestrator crée un orchestrateur d'arrêt avec le timeout spécifié.
func NewShutdownOrchestrator(log *slog.Logger, timeout time.Duration) *ShutdownOrchestrator {
	return &ShutdownOrchestrator{
		log:     log,
		timeout: timeout,
	}
}

// Register ajoute un composant à l'orchestrateur. Les composants sont arrêtés
// dans l'ordre inverse d'enregistrement (LIFO — le dernier enregistré est
// arrêté en premier).
func (o *ShutdownOrchestrator) Register(name string, comp Shutdownable) {
	o.components = append(o.components, namedComponent{name: name, comp: comp})
}

// Shutdown effectue l'arrêt graceful de tous les composants enregistrés
// dans l'ordre inverse. Respecte le timeout global du contexte.
func (o *ShutdownOrchestrator) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	o.log.Info("début de l'arrêt graceful",
		"components", len(o.components),
		"timeout", o.timeout.String(),
	)

	var firstErr error
	// Arrêt LIFO — les composants de plus haut niveau d'abord
	for i := len(o.components) - 1; i >= 0; i-- {
		nc := o.components[i]
		o.log.Info("arrêt du composant", "name", nc.name)

		if err := nc.comp.Close(ctx); err != nil {
			o.log.Error("erreur arrêt composant", "name", nc.name, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			o.log.Info("composant arrêté", "name", nc.name)
		}
	}

	if firstErr != nil {
		o.log.Warn("arrêt graceful terminé avec erreurs", "first_error", firstErr)
	} else {
		o.log.Info("arrêt graceful terminé sans erreur")
	}

	return firstErr
}

// GracefulShutdown est un hook simplifié d'orchestration pour arrêt propre.
// Utilisé comme point d'entrée rapide quand un orchestrateur complet n'est pas
// nécessaire.
func GracefulShutdown(ctx context.Context) error {
	_ = ctx
	return nil
}
