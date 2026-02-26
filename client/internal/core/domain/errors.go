// Package domain — errors.go
//
// Définit les erreurs sentinelles du client ZTNA. Ces erreurs sont
// utilisées par les différentes couches pour signaler des conditions
// d'erreur typées, permettant un traitement différencié par l'appelant.
package domain

import "errors"

var (
	// ErrNotAuthenticated indique que l'utilisateur n'est pas encore
	// authentifié (pas de token stocké ou token expiré sans refresh possible).
	ErrNotAuthenticated = errors.New("non authentifié : exécutez 'ztna login'")

	// ErrTokenExpired indique que l'access_token est expiré et que le
	// rafraîchissement a échoué.
	ErrTokenExpired = errors.New("token expiré : exécutez 'ztna login'")

	// ErrNoCertificate indique qu'aucun certificat mTLS client n'est
	// disponible localement.
	ErrNoCertificate = errors.New("pas de certificat mTLS : exécutez 'ztna cert'")

	// ErrCertExpired indique que le certificat mTLS client a expiré.
	ErrCertExpired = errors.New("certificat mTLS expiré : exécutez 'ztna cert'")

	// ErrConnectionDenied indique que la Gateway a refusé la connexion
	// (décision "deny" du Control Plane).
	ErrConnectionDenied = errors.New("connexion refusée par la politique d'accès")

	// ErrGatewayUnreachable indique que la Gateway n'est pas joignable.
	ErrGatewayUnreachable = errors.New("gateway inaccessible")

	// ErrControlPlaneUnreachable indique que le Control Plane n'est pas joignable.
	ErrControlPlaneUnreachable = errors.New("control plane inaccessible")

	// ErrInvalidConfig indique une erreur dans la configuration.
	ErrInvalidConfig = errors.New("configuration invalide")

	// ErrProtocolMismatch indique une incompatibilité de protocole avec la Gateway.
	ErrProtocolMismatch = errors.New("incompatibilité de protocole avec la gateway")
)
