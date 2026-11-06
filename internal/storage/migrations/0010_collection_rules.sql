-- Collection rules: checks across requests evaluated after a collection run
-- (docs/collection-rules.md). Stored with the collection so they are exported,
-- duplicated and imported with it.
ALTER TABLE collections ADD COLUMN rules_json TEXT NOT NULL DEFAULT '[]';
