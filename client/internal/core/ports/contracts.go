// Package ports expose les contrats entre usecases et implémentations infra.
//
// L'objectif est de garder les workflows client testables et remplaçables
// (mocks unitaires, adaptateurs différents selon environnement).
package ports

import (
	"context"
	"net"
	"time"

	"client/internal/core/domain"
)

// LoginResult représente le résultat minimal d'une authentification OIDC.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Subject      domain.SubjectRef
}

// IdentityProvider gère l'authentification auprès du fournisseur OIDC.
type IdentityProvider interface {
	LoginPasswordGrantLAB(ctx context.Context, username, password string) (*LoginResult, error)
	DeviceFlowLogin(ctx context.Context) (*LoginResult, error)
}

// TokenProvider fournit un access token valide pour les appels sécurisés.
type TokenProvider interface {
	GetValidAccessToken(ctx context.Context) (string, error)
}

// CertificateIssuer demande un certificat mTLS au Control Plane.
type CertificateIssuer interface {
	RequestMTLSCertFromCP(accessToken string) error
}

// CertificateStore persiste et charge les artefacts mTLS du client.
type CertificateStore interface {
	SaveCertAndKey(certPEM, keyPEM []byte) error
	LoadCertAndKey() (certPEM, keyPEM []byte, err error)
}

// ResourceCatalog résout un nom logique en endpoint réseau concret.
//
// Exemple: "ssh-backend" -> {Type: "ssh", Host: "10.0.20.10", Port: 22}
type ResourceCatalog interface {
	Resolve(ctx context.Context, resourceName string) (domain.ResourceRef, error)
}

// TunnelConnector établit et opère un tunnel mTLS vers la Gateway.
type TunnelConnector interface {
	Connect(certPEM, keyPEM []byte, resource string) (net.Conn, error)
	RelayTraffic(tunnel net.Conn, local net.Conn) error
}

// LocalEndpointDialer ouvre la connexion locale à relayer dans le tunnel.
//
// TODO: définir une stratégie claire par type de ressource (ssh/tcp/http).
type LocalEndpointDialer interface {
	Open(ctx context.Context, resource domain.ResourceRef) (net.Conn, error)
}

// DeviceContextProvider collecte le contexte terminal/OS/réseau transmis
// à la Gateway pour enrichir les décisions d'accès.
type DeviceContextProvider interface {
	Build(ctx context.Context) (domain.RequestContext, error)
}
