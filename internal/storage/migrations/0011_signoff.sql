-- Release sign-off report (docs/signoff-report.md): who approved a baseline.
-- Empty for baselines approved before this version.
ALTER TABLE baselines ADD COLUMN approved_by TEXT NOT NULL DEFAULT '';
