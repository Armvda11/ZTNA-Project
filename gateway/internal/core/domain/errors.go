// Package domain — errors.go
//
// Erreurs sentinelles de la Gateway ZTNA.
package domain

import "errors"

var (
	// ErrAccessDenied indique que le Control Plane a refusé l'accès.
	ErrAccessDenied = errors.New("accès refusé par le control plane")

	// ErrInvalidCert indique un problème avec le certificat client.
	ErrInvalidCert = errors.New("certificat client invalide")

	// ErrNoIdentity indique que l'identité n'a pas pu être extraite du certificat.
	ErrNoIdentity = errors.New("impossible d'extraire l'identité du certificat")

	// ErrInvalidRequest indique une requête CONNECT malformée.
	ErrInvalidRequest = errors.New("requête CONNECT invalide")

	// ErrTargetUnreachable indique que la ressource cible n'est pas joignable.
	ErrTargetUnreachable = errors.New("ressource cible inaccessible")

	// ErrControlPlaneUnreachable indique que le CP n'est pas joignable.
	ErrControlPlaneUnreachable = errors.New("control plane inaccessible")

	// ErrTooManySessions indique que la limite de sessions a été atteinte.
	ErrTooManySessions = errors.New("limite de sessions atteinte")

	// ErrSessionExpired indique que la session a dépassé son TTL.
	ErrSessionExpired = errors.New("session expirée")

	// ErrProtocolError indique une erreur de protocole CONNECT.
	ErrProtocolError = errors.New("erreur de protocole")
)
