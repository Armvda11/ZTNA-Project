-- 004_sessions.sql
-- Table de télémétrie des sessions TCP relayées par la gateway (PEP).
-- Chaque ligne correspond à une session ouverte : elle est créée à l'ouverture
-- (/pep/sessions/start) et complétée à la fermeture (/pep/sessions/end).
-- La corrélation avec la décision PDP se fait via decision_id.

CREATE TABLE IF NOT EXISTS sessions (
    session_id        TEXT    PRIMARY KEY,
    decision_id       TEXT    NOT NULL,          -- ref. vers audit_events
    subject_sub       TEXT    NOT NULL DEFAULT '',
    subject_username  TEXT    NOT NULL DEFAULT '',
    device_serial     TEXT    NOT NULL DEFAULT '',
    resource_type     TEXT    NOT NULL DEFAULT '',
    resource_match    TEXT    NOT NULL DEFAULT '',
    start_time        TEXT    NOT NULL,          -- RFC3339 UTC
    end_time          TEXT,                      -- NULL tant que la session est active
    bytes_in          INTEGER NOT NULL DEFAULT 0,
    bytes_out         INTEGER NOT NULL DEFAULT 0,
    duration_ms       INTEGER NOT NULL DEFAULT 0,
    end_reason        TEXT    NOT NULL DEFAULT '' -- "eof" | "error" | "revoked" | "dial_error"
);

CREATE INDEX IF NOT EXISTS idx_sessions_decision ON sessions(decision_id);
CREATE INDEX IF NOT EXISTS idx_sessions_device   ON sessions(device_serial);
CREATE INDEX IF NOT EXISTS idx_sessions_start    ON sessions(start_time);
