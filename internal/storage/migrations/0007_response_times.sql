-- Response-time trends: group history by environment ID (the name can change)
-- and leave race tests and permission matrices out of trends, because their
-- concurrent requests are slower by design.
ALTER TABLE runs ADD COLUMN environment_id TEXT;
ALTER TABLE runs ADD COLUMN kind TEXT NOT NULL DEFAULT 'run';

-- Older race tests and matrices were saved as ordinary runs. They are the only
-- runs that send one request more than once.
UPDATE runs SET kind = 'burst' WHERE id IN (
    SELECT run_id FROM run_results WHERE request_id IS NOT NULL
    GROUP BY run_id, request_id HAVING COUNT(*) > 1);

ALTER TABLE run_results ADD COLUMN performance_json TEXT;

CREATE TABLE performance_checks (
    request_id   TEXT PRIMARY KEY REFERENCES requests(id) ON DELETE CASCADE,
    config_json  TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
