// Package credentials gère la demande de certificats mTLS client auprès
// du Control Plane. Le client génère localement une paire de clés, construit
// un CSR et l'envoie au CP qui retourne un certificat X.509 signé de courte
// durée de vie.
//
// IMPORTANT: L'endpoint CP POST /api/v1/credentials/mtls-cert N'EXISTE PAS
// ENCORE. Il sera implémenté ultérieurement dans le Control Plane.
// Ce package est un skeleton/stub en attendant.
package credentials

import (
	"fmt"
	"log/slog"

	"client/internal/config"
)

// Client est le client de gestion des certificats mTLS.
type Client struct {
	cfg *config.Config
	log *slog.Logger
}

// NewClient crée un nouveau client de gestion de certificats.
func NewClient(cfg *config.Config, log *slog.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

// RequestMTLSCertFromCP demande un certificat mTLS client au Control Plane.
//
// Flux prévu :
//  1. Générer une paire de clés ECDSA P-256 localement
//  2. Construire un CSR (Certificate Signing Request) avec :
//     - Subject CN = sub (identifiant OIDC de l'utilisateur)
//     - SAN URI = oidc:{sub} (optionnel, pour identification dans la Gateway)
//  3. Encoder le CSR en PEM
//  4. Appeler POST /api/v1/credentials/mtls-cert sur le Control Plane :
//     - Header: Authorization: Bearer {accessToken}
//     - Body: { "csr": "<PEM-encoded CSR>" }
//  5. Recevoir la réponse : { "certificate": "<PEM-encoded cert>", "expires_at": "..." }
//  6. Sauvegarder le certificat et la clé privée via SaveCertAndKey()
//
// SÉCURITÉ CRITIQUE :
//   - La clé privée ne quitte JAMAIS le client
//   - Le CP ne voit que le CSR (clé publique + métadonnées)
//   - Le certificat doit être de courte durée (15 min par défaut)
//   - Le client doit redemander un certificat avant expiration
//
// TODO: Implémenter quand l'endpoint CP sera disponible
// TODO: Ajouter la validation du certificat reçu (chaîne de confiance, dates)
// TODO: Ajouter un mécanisme de renouvellement automatique
func (c *Client) RequestMTLSCertFromCP(accessToken string) error {
	c.log.Info("demande de certificat mTLS au Control Plane")

	// TODO: générer la paire de clés ECDSA P-256
	//   key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// TODO: construire le CSR
	//   template := &x509.CertificateRequest{
	//       Subject: pkix.Name{CommonName: "<sub from token>"},
	//   }
	//   csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, key)

	// TODO: encoder le CSR en PEM
	//   csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// TODO: appeler POST {control_plane.base_url}/api/v1/credentials/mtls-cert
	//   - Header: Authorization: Bearer {accessToken}
	//   - Body JSON: { "csr": string(csrPEM) }
	//   - TLS: utiliser control_plane.ca_file si configuré

	// TODO: parser la réponse et extraire le certificat PEM

	// TODO: appeler SaveCertAndKey(certPEM, keyPEM)

	return fmt.Errorf("TODO: RequestMTLSCertFromCP non implémenté (endpoint CP inexistant)")
}

// SaveCertAndKey sauvegarde le certificat et la clé privée dans le
// répertoire de stockage configuré.
//
// TODO: Implémenter la sauvegarde avec permissions strictes :
//   - Certificat : {storage.path}/client.crt (permissions 0644)
//   - Clé privée : {storage.path}/client.key (permissions 0600)
//   - Encoder en PEM
//   - Vérifier que le certificat correspond à la clé privée
func (c *Client) SaveCertAndKey(certPEM, keyPEM []byte) error {
	c.log.Info("sauvegarde du certificat et de la clé privée")

	// TODO: écrire certPEM dans {storage.path}/client.crt avec 0644
	// TODO: écrire keyPEM dans {storage.path}/client.key avec 0600
	// TODO: valider que le cert correspond à la clé (x509.Certificate.PublicKey)

	return fmt.Errorf("TODO: SaveCertAndKey non implémenté")
}

// LoadCertAndKey charge le certificat et la clé privée depuis le stockage.
//
// TODO: Vérifier l'expiration du certificat avant de le retourner
// TODO: Si expiré, retourner une erreur claire invitant à relancer 'ztna cert'
func (c *Client) LoadCertAndKey() (certPEM, keyPEM []byte, err error) {
	c.log.Debug("chargement du certificat mTLS client")

	// TODO: lire {storage.path}/client.crt et {storage.path}/client.key
	// TODO: parser le certificat et vérifier l'expiration
	// TODO: retourner les données PEM

	return nil, nil, fmt.Errorf("TODO: LoadCertAndKey non implémenté")
}
