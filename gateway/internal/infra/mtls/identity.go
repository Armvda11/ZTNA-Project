// Package mtls — identity.go
//
// Extraction de l'identité du sujet à partir d'un certificat X.509 client.
// L'identité est utilisée pour construire la requête d'autorisation envoyée
// au Control Plane.
//
// Stratégie d'extraction (par ordre de priorité) :
//  1. SAN URI : si le certificat contient un SAN de type URI (ex: oidc:<sub>),
//     le "sub" est extrait depuis l'URI
//  2. SAN DNS : utilisé comme identifiant si présent
//  3. Subject CN (Common Name) : fallback classique
//
// NOTE: Les certificats mTLS clients sont de courte durée (15 min) et émis
// par le Control Plane. Le CN est typiquement le "sub" OIDC de l'utilisateur.
// La Gateway fait confiance à cette information car le certificat est signé
// par la CA du CP.
package mtls

import (
	"crypto/x509"
	"log/slog"
	"net/url"
	"strings"

	"ztna-gateway/internal/core/domain"
)

// ExtractSubjectFromCert extrait les informations d'identité du sujet
// depuis un certificat X.509 client vérifié.
//
// L'extraction suit la priorité :
//  1. SAN URI avec schéma "oidc:" → Sub = partie spécifique de l'URI
//  2. Premier SAN DNS → Sub = DNS name
//  3. Subject CN → Sub = Common Name
//
// TODO: Enrichir avec les groupes si présents dans les extensions du certificat
// TODO: Ajouter la validation du format de l'identifiant extrait
func ExtractSubjectFromCert(cert *x509.Certificate, log *slog.Logger) domain.SubjectRef {
	subject := domain.SubjectRef{}

	// Priorité 1 : SAN URI (ex: oidc:user-uuid-1234)
	for _, uri := range cert.URIs {
		if uri.Scheme == "oidc" {
			subject.Sub = uri.Opaque
			if subject.Sub == "" {
				// Fallback : essayer le host + path
				subject.Sub = strings.TrimPrefix(uri.String(), "oidc:")
			}
			log.Debug("identité extraite depuis SAN URI", "sub", subject.Sub)
			break
		}
	}

	// Priorité 2 : SAN DNS
	if subject.Sub == "" && len(cert.DNSNames) > 0 {
		subject.Sub = cert.DNSNames[0]
		log.Debug("identité extraite depuis SAN DNS", "sub", subject.Sub)
	}

	// Priorité 3 : Subject CN
	if subject.Sub == "" && cert.Subject.CommonName != "" {
		subject.Sub = cert.Subject.CommonName
		log.Debug("identité extraite depuis Subject CN", "sub", subject.Sub)
	}

	// Extraire le username depuis le Subject CN si différent du sub
	if cert.Subject.CommonName != "" && cert.Subject.CommonName != subject.Sub {
		subject.Username = cert.Subject.CommonName
	} else {
		subject.Username = subject.Sub
	}

	// Extraire les groupes depuis le champ Organization du certificat X.509.
	// Le CP encode les groupes OIDC dans le champ Organization lors de
	// l'émission du certificat mTLS client.
	if len(cert.Subject.Organization) > 0 {
		subject.Groups = make([]string, len(cert.Subject.Organization))
		copy(subject.Groups, cert.Subject.Organization)
		log.Debug("groupes extraits du certificat",
			"groups", subject.Groups,
			"source", "X.509 Organization",
		)
	}

	return subject
}

// parseOIDCURI parse une URI de type "oidc:subject-id" et retourne
// l'identifiant du sujet.
func parseOIDCURI(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme != "oidc" {
		return ""
	}
	return strings.TrimPrefix(raw, "oidc:")
}
