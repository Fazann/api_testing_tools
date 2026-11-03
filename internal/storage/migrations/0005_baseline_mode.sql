-- Baselines can compare values (as before) or only the response structure.
ALTER TABLE baselines ADD COLUMN mode TEXT NOT NULL DEFAULT 'values';
