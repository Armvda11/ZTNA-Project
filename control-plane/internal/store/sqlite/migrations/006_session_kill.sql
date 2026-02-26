-- 006_session_kill.sql
-- Ajout de la capacité de "kill session" côté Control Plane.
-- Un admin peut marquer une session active comme "killed" via
-- DELETE /api/v1/admin/sessions/{id}.
-- La Gateway interroge GET /api/v1/pep/sessions/{id}/valid toutes les 5s
-- et coupe le proxy si killed_at IS NOT NULL.

ALTER TABLE sessions ADD COLUMN killed_at TEXT; -- RFC3339 UTC, NULL si session non tuée
ALTER TABLE sessions ADD COLUMN killed_by TEXT; -- sub OIDC de l'admin qui a tué la session

CREATE INDEX IF NOT EXISTS idx_sessions_killed ON sessions(killed_at) WHERE killed_at IS NOT NULL;
