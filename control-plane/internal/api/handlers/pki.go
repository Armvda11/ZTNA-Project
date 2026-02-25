package handlers

import (
	"net/http"

	"control-plane/internal/service/credentials"
)

// PKIHandler serves public key infrastructure endpoints (CA cert, CRL, SSH CA pubkey).
// These endpoints are unauthenticated so that gateways can bootstrap their
// trust store without prior credentials.
type PKIHandler struct {
	deviceCert *credentials.DeviceCertService
	sshCreds   *credentials.Service
}

// NewPKIHandler creates the PKI handler.
func NewPKIHandler(svc *credentials.DeviceCertService, sshCreds *credentials.Service) *PKIHandler {
	return &PKIHandler{deviceCert: svc, sshCreds: sshCreds}
}

// CACert handles GET /pki/device-ca/cert
// Returns the PEM-encoded Device CA certificate. No authentication required.
func (h *PKIHandler) CACert(w http.ResponseWriter, r *http.Request) {
	pem := h.deviceCert.CACertPEM()
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pem)
}

// CRL handles GET /pki/device-ca/crl
// Returns the DER-encoded CRL for the Device CA. No authentication required.
// The CRL is regenerated on each request (short-lived, not cached here).
func (h *PKIHandler) CRL(w http.ResponseWriter, r *http.Request) {
	crlDER, err := h.deviceCert.GenerateCRL(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/pkix-crl")
	w.Header().Set("Cache-Control", "public, max-age=300") // 5 min
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(crlDER)
}

// SSHCAPubKey handles GET /pki/ssh-ca/pubkey
// Returns the SSH CA public key in authorized_keys format, suitable for use
// as the TrustedUserCAKeys entry on SSH servers (gateway, lan-app). No auth required.
func (h *PKIHandler) SSHCAPubKey(w http.ResponseWriter, r *http.Request) {
	pubKey := h.sshCreds.CAPubKeyAuthorizedKey()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pubKey)
}
