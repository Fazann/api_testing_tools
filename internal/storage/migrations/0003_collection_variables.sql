-- Collection-level variables (base_url, token, ...) shared by every request.
ALTER TABLE collections ADD COLUMN variables_json TEXT NOT NULL DEFAULT '[]';
