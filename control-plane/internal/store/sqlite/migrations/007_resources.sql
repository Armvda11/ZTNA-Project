-- Published resources: centrally-managed ZTNA resources.
-- A resource that does not exist in this table does not exist in the system.
CREATE TABLE IF NOT EXISTS resources (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    type       TEXT NOT NULL CHECK(type IN ('web','ssh','db')),
    backend    TEXT NOT NULL,
    gateway_id TEXT NOT NULL DEFAULT '',
    group_match TEXT NOT NULL DEFAULT '[]',   -- JSON array of group names
    access_mode TEXT NOT NULL CHECK(access_mode IN ('http-proxy','ssh-cert','tcp-tunnel')),
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_name ON resources(name);
