-- Add gateway metadata used for registration proof and inventory.
ALTER TABLE gateways ADD COLUMN fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE gateways ADD COLUMN software_version TEXT NOT NULL DEFAULT '';

