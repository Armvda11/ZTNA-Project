-- Migration 006: Ajout des conditions contextuelles aux règles de politique.
-- Ces colonnes sont optionnelles (nullable) et n'affectent pas les règles existantes.
ALTER TABLE policy_rules ADD COLUMN allowed_hours TEXT NOT NULL DEFAULT '';
ALTER TABLE policy_rules ADD COLUMN required_device_trust TEXT NOT NULL DEFAULT '';
