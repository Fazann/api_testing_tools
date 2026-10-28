-- Collection-level auth that requests can inherit ("type": "inherit").
ALTER TABLE collections ADD COLUMN auth_json TEXT NOT NULL DEFAULT '{"type":"none"}';
