-- Race-condition test settings per request and the permission matrix per
-- collection. Both are stored as JSON documents and deleted with their owner.
CREATE TABLE race_tests (
    request_id   TEXT PRIMARY KEY REFERENCES requests(id) ON DELETE CASCADE,
    config_json  TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE permission_matrices (
    collection_id  TEXT PRIMARY KEY REFERENCES collections(id) ON DELETE CASCADE,
    config_json    TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
