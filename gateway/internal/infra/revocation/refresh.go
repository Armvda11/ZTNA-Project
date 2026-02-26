package crl

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// RefreshFromCP fetch the CRL from the Control Plane and updates the store.
// cpBaseURL: e.g. "https://10.10.20.30:8080"
// httpClient: pre-configured HTTP client (skip TLS verify for lab)
func (s *Store) RefreshFromCP(ctx context.Context, cpBaseURL string, httpClient *http.Client, log *slog.Logger) error {
	url := strings.TrimRight(cpBaseURL, "/") + "/pki/device-ca/crl"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build CRL request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch CRL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CRL endpoint returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read CRL body: %w", err)
	}

	// Parse DER-encoded CRL
	crl, err := x509.ParseRevocationList(body)
	if err != nil {
		return fmt.Errorf("parse CRL DER: %w", err)
	}

	serials := make([]string, 0, len(crl.RevokedCertificateEntries))
	for _, entry := range crl.RevokedCertificateEntries {
		// Consistent format with CP: lowercase hex (fmt.Sprintf("%x", big.Bytes()))
		serial := fmt.Sprintf("%x", entry.SerialNumber.Bytes())
		serials = append(serials, serial)
	}

	s.Replace(serials)

	if log != nil {
		log.Debug("CRL refreshed", "revoked_count", len(serials), "url", url)
	}
	return nil
}

// StartAutoRefresh starts a background goroutine that periodically fetches the CRL.
func (s *Store) StartAutoRefresh(ctx context.Context, cpBaseURL string, httpClient *http.Client, interval time.Duration, log *slog.Logger) {
	go func() {
		// Initial refresh immediately
		if err := s.RefreshFromCP(ctx, cpBaseURL, httpClient, log); err != nil {
			if log != nil {
				log.Warn("CRL initial refresh failed", "error", err)
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.RefreshFromCP(ctx, cpBaseURL, httpClient, log); err != nil {
					if log != nil {
						log.Warn("CRL refresh failed", "error", err)
					}
				}
			}
		}
	}()
}
