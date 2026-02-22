-- Device X.509 certificates issued by the Device CA for mTLS user<->gateway.
CREATE TABLE IF NOT EXISTS device_certs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    serial TEXT NOT NULL UNIQUE,
    sub TEXT NOT NULL,
    username TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked INTEGER NOT NULL DEFAULT 0,
    revoked_at TEXT,
    revocation_reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_device_certs_sub ON device_certs(sub);
CREATE INDEX IF NOT EXISTS idx_device_certs_revoked ON device_certs(revoked);
