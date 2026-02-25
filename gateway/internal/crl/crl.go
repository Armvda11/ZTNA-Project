// Package crl fournit un cache en mémoire de la CRL (Certificate Revocation List)
// de la Device CA, rafraîchi périodiquement depuis le Control Plane.
//
// Design :
//   - Le set des serials révoqués est stocké dans un atomic.Pointer[map] pour
//     des lectures lock-free en O(1) sur chaque connexion entrante.
//   - Un goroutine de refresh tourne en arrière-plan ; si un refresh échoue,
//     la dernière CRL connue est conservée (pas de fail-open/fail-closed brutal).
//   - Le callback onRefresh est appelé après chaque refresh réussi pour permettre
//     au proxy de couper les sessions dont le cert vient d'être révoqué.
package crl

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"sync/atomic"
	"time"
)

// CRLCache maintient un set en mémoire des numéros de série révoqués.
type CRLCache struct {
	revoked atomic.Pointer[map[string]struct{}]
	log     *slog.Logger
}

// New crée un CRLCache vide (fail-open au boot jusqu'au premier refresh réussi).
func New(log *slog.Logger) *CRLCache {
	c := &CRLCache{log: log}
	empty := make(map[string]struct{})
	c.revoked.Store(&empty)
	return c
}

// SerialHex encode un *big.Int (SerialNumber X.509) en hex minuscule sans préfixe.
// Même encodage que la CRL pour garantir la cohérence des clés.
func SerialHex(n *big.Int) string {
	if n == nil {
		return "00"
	}
	b := n.Bytes()
	if len(b) == 0 {
		return "00"
	}
	return hex.EncodeToString(b)
}

// IsRevoked retourne true si le serial du certificat figure dans la CRL courante.
// Thread-safe, O(1), pas de lock.
func (c *CRLCache) IsRevoked(serial *big.Int) bool {
	m := c.revoked.Load()
	_, ok := (*m)[SerialHex(serial)]
	return ok
}

// IsRevokedHex retourne true si le serial (hex minuscule) figure dans la CRL courante.
// Utilisé par le SessionRegistry qui indexe par chaîne hex.
func (c *CRLCache) IsRevokedHex(hexSerial string) bool {
	m := c.revoked.Load()
	_, ok := (*m)[hexSerial]
	return ok
}

// RevokedSerials retourne une copie du set courant des serials révoqués.
// Utile pour audit ou tests.
func (c *CRLCache) RevokedSerials() map[string]struct{} {
	m := c.revoked.Load()
	cp := make(map[string]struct{}, len(*m))
	for k := range *m {
		cp[k] = struct{}{}
	}
	return cp
}

// FetchAndUpdate télécharge la CRL depuis le CP (format DER), la parse et met
// à jour atomiquement le set en mémoire. Retourne une erreur si le fetch ou le
// parsing échoue ; dans ce cas le set courant est inchangé.
func (c *CRLCache) FetchAndUpdate(cpURL string, client *http.Client) error {
	resp, err := client.Get(cpURL + "/pki/device-ca/crl")
	if err != nil {
		return fmt.Errorf("fetch crl: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch crl: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read crl body: %w", err)
	}

	rl, err := x509.ParseRevocationList(body)
	if err != nil {
		return fmt.Errorf("parse crl der: %w", err)
	}

	newSet := make(map[string]struct{}, len(rl.RevokedCertificateEntries))
	for _, entry := range rl.RevokedCertificateEntries {
		newSet[SerialHex(entry.SerialNumber)] = struct{}{}
	}

	c.revoked.Store(&newSet)
	c.log.Info("crl refreshed", slog.Int("revoked_count", len(newSet)))
	return nil
}

// StartRefreshLoop lance un goroutine qui rafraîchit la CRL toutes les interval.
// Si un refresh échoue, la dernière CRL connue est conservée (log warn).
// onRefresh est appelé après chaque refresh réussi (nil = ignoré) — typiquement
// utilisé pour couper les sessions actives dont le cert vient d'être révoqué.
func (c *CRLCache) StartRefreshLoop(
	ctx context.Context,
	interval time.Duration,
	cpURL string,
	client *http.Client,
	onRefresh func(),
	strictStartup bool,
) error {
	if interval <= 0 {
		interval = 60 * time.Second
	}

	// Refresh immédiat au démarrage pour ne pas attendre le premier tick.
	if err := c.FetchAndUpdate(cpURL, client); err != nil {
		if strictStartup {
			return fmt.Errorf("initial CRL fetch failed in strict mode: %w", err)
		}
		c.log.Warn("crl initial fetch failed, starting with empty crl (fail-open)",
			slog.Any("err", err))
	} else if onRefresh != nil {
		onRefresh()
	}

	go func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if err := c.FetchAndUpdate(cpURL, client); err != nil {
					c.log.Warn("crl refresh failed, keeping last known crl",
						slog.Any("err", err))
				} else if onRefresh != nil {
					onRefresh()
				}
			}
		}
	}()
	return nil
}
