-- Form-data / URL-encoded fields and the binary body file of a request.
ALTER TABLE requests ADD COLUMN form_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE requests ADD COLUMN body_file TEXT NOT NULL DEFAULT '';
