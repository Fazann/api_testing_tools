-- Pre-request and post-response scripts on requests and collections, and the
-- console output they wrote during a run.
ALTER TABLE requests ADD COLUMN pre_request_script TEXT NOT NULL DEFAULT '';
ALTER TABLE requests ADD COLUMN post_response_script TEXT NOT NULL DEFAULT '';
ALTER TABLE collections ADD COLUMN pre_request_script TEXT NOT NULL DEFAULT '';
ALTER TABLE collections ADD COLUMN post_response_script TEXT NOT NULL DEFAULT '';
ALTER TABLE run_results ADD COLUMN logs_json TEXT NOT NULL DEFAULT '[]';
