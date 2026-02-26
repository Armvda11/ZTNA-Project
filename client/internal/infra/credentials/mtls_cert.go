// Package credentials gère la demande de certificats mTLS client auprès
// du Control Plane. Le client génère localement une paire de clés, construit
// un CSR et l'envoie au CP qui retourne un certificat X.509 signé de courte
// durée de vie.
//
// L'endpoint CP POST /api/v1/credentials/mtls-cert est appelé avec un
// Bearer token OIDC. Le CP valide le token, signe le CSR avec sa Device CA,
// et retourne le certificat PEM au client.
package credentials

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"client/internal/config"
	"client/internal/core/domain"
	"client/internal/infra/storage"
	tlsinfra "client/internal/infra/tls"
)

// Client est le client de gestion des certificats mTLS.
type Client struct {
	cfg       *config.Config
	log       *slog.Logger
	http      *http.Client
	certStore *storage.CertFileStore
}

// NewClient crée un nouveau client de gestion de certificats.
func NewClient(cfg *config.Config, log *slog.Logger) *Client {
	httpClient, err := tlsinfra.NewControlPlaneHTTPClient(cfg)
	if err != nil {
		log.Warn("client HTTP TLS indisponible pour credentials, fallback HTTP standard", "error", err)
		httpClient = http.DefaultClient
	}

	return &Client{
		cfg:       cfg,
		log:       log,
		http:      httpClient,
		certStore: storage.NewCertFileStore(cfg, log),
	}
}

// certRequest est le corps JSON envoyé au CP pour demander un certificat.
type certRequest struct {
	CSR string `json:"csr"`
}

// certResponse est la réponse JSON du CP contenant le certificat signé.
type certResponse struct {
	Certificate string `json:"certificate"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

// RequestMTLSCertFromCP demande un certificat mTLS client au Control Plane.
//
// Flux :
//  1. Générer une paire de clés ECDSA P-256 localement
//  2. Construire un CSR (Certificate Signing Request)
//  3. Appeler POST /api/v1/credentials/mtls-cert avec Bearer token
//  4. Sauvegarder le certificat reçu et la clé privée
//
// SÉCURITÉ CRITIQUE :
//   - La clé privée ne quitte JAMAIS le client
//   - Le CP ne voit que le CSR (clé publique + métadonnées)
func (c *Client) RequestMTLSCertFromCP(accessToken string) error {
	c.log.Info("demande de certificat mTLS au Control Plane")

	// 1. Générer la paire de clés ECDSA P-256
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("impossible de générer la paire de clés ECDSA: %w", err)
	}

	// 2. Construire le CSR
	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "ztna-client",
			Organization: []string{"ZTNA"},
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, key)
	if err != nil {
		return fmt.Errorf("impossible de créer le CSR: %w", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// 3. Encoder la clé privée en PEM
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("impossible d'encoder la clé privée: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	// 4. Appeler l'endpoint du Control Plane
	reqBody := certRequest{CSR: string(csrPEM)}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("impossible de sérialiser la requête: %w", err)
	}

	cpURL := strings.TrimRight(c.cfg.ControlPlane.BaseURL, "/") + "/api/v1/credentials/mtls-cert"
	httpReq, err := http.NewRequest(http.MethodPost, cpURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("impossible de construire la requête HTTP: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: impossible de joindre le Control Plane: %v",
			domain.ErrControlPlaneUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("échec de demande de certificat (%d): %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// 5. Parser la réponse
	var certResp certResponse
	if err := json.NewDecoder(resp.Body).Decode(&certResp); err != nil {
		return fmt.Errorf("impossible de parser la réponse du CP: %w", err)
	}
	if certResp.Certificate == "" {
		return fmt.Errorf("réponse du CP invalide: certificat manquant")
	}

	certPEM := []byte(certResp.Certificate)

	// 6. Valider que le certificat correspond à notre clé privée
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("le certificat reçu ne correspond pas à la clé générée: %w", err)
	}

	// 7. Sauvegarder le certificat et la clé
	if err := c.certStore.SaveCertAndKey(certPEM, keyPEM); err != nil {
		return fmt.Errorf("impossible de sauvegarder le certificat: %w", err)
	}

	c.log.Info("certificat mTLS obtenu et sauvegardé avec succès")
	return nil
}

// SaveCertAndKey sauvegarde le certificat et la clé privée dans le
// répertoire de stockage configuré, après validation de la cohérence.
func (c *Client) SaveCertAndKey(certPEM, keyPEM []byte) error {
	c.log.Info("sauvegarde du certificat et de la clé privée")

	// Valider que le certificat correspond à la clé privée
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("certificat et clé ne correspondent pas: %w", err)
	}

	return c.certStore.SaveCertAndKey(certPEM, keyPEM)
}

// LoadCertAndKey charge le certificat et la clé privée depuis le stockage.
// Vérifie que le certificat n'est pas expiré avant de le retourner.
func (c *Client) LoadCertAndKey() (certPEM, keyPEM []byte, err error) {
	c.log.Debug("chargement du certificat mTLS client")

	certPEM, keyPEM, err = c.certStore.LoadCertAndKey()
	if err != nil {
		return nil, nil, err
	}

	// Vérifier l'expiration du certificat
	block, _ := pem.Decode(certPEM)
	if block != nil {
		cert, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr == nil && time.Now().After(cert.NotAfter) {
			return nil, nil, domain.ErrCertExpired
		}
	}

	return certPEM, keyPEM, nil
}
