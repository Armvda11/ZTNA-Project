package model

// DeviceCert represents an X.509 client certificate issued by the Device CA
// for mTLS authentication between a client and a gateway.
type DeviceCert struct {
	ID               int64  `json:"id"`
	Serial           string `json:"serial"`
	Sub              string `json:"sub"`
	Username         string `json:"username"`
	Fingerprint      string `json:"fingerprint"`
	IssuedAt         string `json:"issued_at"`
	ExpiresAt        string `json:"expires_at"`
	Revoked          bool   `json:"revoked"`
	RevokedAt        string `json:"revoked_at,omitempty"`
	RevocationReason string `json:"revocation_reason,omitempty"`
}
