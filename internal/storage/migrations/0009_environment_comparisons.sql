-- Environment-comparison settings per collection (environments A and B,
-- ignored paths, options), stored as one JSON document and deleted with the
-- collection. Comparison results are not stored.
CREATE TABLE environment_comparisons (
    collection_id  TEXT PRIMARY KEY REFERENCES collections(id) ON DELETE CASCADE,
    config_json    TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
