-- Registered gateway (PEP) instances.
CREATE TABLE IF NOT EXISTS gateways (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    registered_at TEXT NOT NULL,
    last_seen TEXT,
    active INTEGER NOT NULL DEFAULT 1
);
