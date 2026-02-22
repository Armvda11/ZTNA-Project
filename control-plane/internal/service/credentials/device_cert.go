package credentials

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"time"

	"control-plane/internal/config"
	"control-plane/internal/crypto/deviceca"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/domain/port"
)

// DeviceCertService issues X.509 client certificates (Device CA) for mTLS
// between ZTNA clients and gateways. It is a separate service from the SSH CA
// service but lives in the same credentials package.
type DeviceCertService struct {
	ca             *deviceca.CA
	cfg            config.DeviceCAConfig
	deviceCertRepo port.DeviceCertRepository
}

// NewDeviceCertService creates the device cert issuance service.
func NewDeviceCertService(
	ca *deviceca.CA,
	cfg config.DeviceCAConfig,
	deviceCertRepo port.DeviceCertRepository,
) *DeviceCertService {
	return &DeviceCertService{ca: ca, cfg: cfg, deviceCertRepo: deviceCertRepo}
}

// IssueDeviceCertRequest holds the parameters for issuing a device certificate.
type IssueDeviceCertRequest struct {
	Subject model.Subject
	CSRPEM  []byte
	TTL     *time.Duration
}

// IssueDeviceCertResponse is the result of a successful device cert issuance.
type IssueDeviceCertResponse struct {
	CertificatePEM []byte
	CACertPEM      []byte
	Serial         string
	ExpiresAt      time.Time
	Fingerprint    string
}

// IssueDeviceCert validates the CSR, signs it with the Device CA, persists
// the metadata and returns the signed certificate.
func (s *DeviceCertService) IssueDeviceCert(
	ctx context.Context,
	req IssueDeviceCertRequest,
) (IssueDeviceCertResponse, error) {
	if len(req.CSRPEM) == 0 {
		return IssueDeviceCertResponse{}, domainErrors.ErrInvalidInput
	}
	if req.Subject.Username == "" {
		return IssueDeviceCertResponse{}, domainErrors.ErrInvalidInput
	}

	// Resolve TTL.
	ttl := parseDurationOrZero(s.cfg.DefaultTTL)
	if ttl == 0 {
		ttl = 7 * 24 * time.Hour
	}
	if req.TTL != nil {
		ttl = *req.TTL
	}

	minTTL := parseDurationOrZero(s.cfg.MinTTL)
	maxTTL := parseDurationOrZero(s.cfg.MaxTTL)
	if minTTL > 0 && ttl < minTTL {
		return IssueDeviceCertResponse{}, fmt.Errorf("%w: ttl below minimum %s", domainErrors.ErrInvalidInput, minTTL)
	}
	if maxTTL > 0 && ttl > maxTTL {
		ttl = maxTTL
	}

	certPEM, fingerprint, err := s.ca.SignClientCSR(
		req.CSRPEM,
		req.Subject.Username,
		req.Subject.Sub,
		req.Subject.Groups,
		ttl,
		s.cfg.AllowedKeyTypes,
	)
	if err != nil {
		if isKeyTypeError(err) {
			return IssueDeviceCertResponse{}, domainErrors.ErrInvalidInput
		}
		return IssueDeviceCertResponse{}, fmt.Errorf("sign device cert: %w", err)
	}

	// Extract serial and expiry from the signed cert.
	serial, expiresAt, err := extractCertMeta(certPEM)
	if err != nil {
		return IssueDeviceCertResponse{}, fmt.Errorf("parse signed cert: %w", err)
	}

	now := time.Now().UTC()
	if err := s.deviceCertRepo.StoreDeviceCert(ctx, model.DeviceCert{
		Serial:      serial,
		Sub:         req.Subject.Sub,
		Username:    req.Subject.Username,
		Fingerprint: fingerprint,
		IssuedAt:    now.Format(time.RFC3339),
		ExpiresAt:   expiresAt.Format(time.RFC3339),
	}); err != nil {
		return IssueDeviceCertResponse{}, fmt.Errorf("store device cert: %w", err)
	}

	return IssueDeviceCertResponse{
		CertificatePEM: certPEM,
		CACertPEM:      s.ca.CACertPEM(),
		Serial:         serial,
		ExpiresAt:      expiresAt,
		Fingerprint:    fingerprint,
	}, nil
}

// CACertPEM returns the PEM-encoded Device CA certificate.
func (s *DeviceCertService) CACertPEM() []byte {
	return s.ca.CACertPEM()
}

// GenerateCRL generates and returns a DER-encoded CRL.
func (s *DeviceCertService) GenerateCRL(ctx context.Context) ([]byte, error) {
	revoked, err := s.deviceCertRepo.ListRevokedDeviceCerts(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]pkix.RevokedCertificate, 0, len(revoked))
	for _, cert := range revoked {
		serial, ok := serialFromHex(cert.Serial)
		if !ok {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, cert.RevokedAt)
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		entries = append(entries, pkix.RevokedCertificate{
			SerialNumber:   serial,
			RevocationTime: ts,
		})
	}

	return s.ca.GenerateCRL(entries)
}

// RevokeDeviceCert marks a device cert as revoked.
func (s *DeviceCertService) RevokeDeviceCert(ctx context.Context, serial, reason string) error {
	return s.deviceCertRepo.RevokeDeviceCert(ctx, serial, reason)
}

// ──────────────────────────────────────────────────────────────
// Internal helpers
// ──────────────────────────────────────────────────────────────

func isKeyTypeError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not allowed")
}

func extractCertMeta(certPEM []byte) (serial string, expiresAt time.Time, err error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", time.Time{}, fmt.Errorf("invalid cert PEM")
	}
	cert, parseErr := x509.ParseCertificate(block.Bytes)
	if parseErr != nil {
		return "", time.Time{}, parseErr
	}
	serial = fmt.Sprintf("%x", cert.SerialNumber.Bytes())
	return serial, cert.NotAfter, nil
}

func serialFromHex(h string) (*big.Int, bool) {
	b := new(big.Int)
	_, ok := b.SetString(h, 16)
	return b, ok
}
