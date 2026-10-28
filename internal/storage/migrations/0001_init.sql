CREATE TABLE collections (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    required_vars_json  TEXT NOT NULL DEFAULT '[]',
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE TABLE folders (
    id             TEXT PRIMARY KEY,
    collection_id  TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    parent_id      TEXT REFERENCES folders(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX idx_folders_collection ON folders(collection_id);
CREATE INDEX idx_folders_parent ON folders(parent_id);

CREATE TABLE requests (
    id                TEXT PRIMARY KEY,
    collection_id     TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    parent_id         TEXT REFERENCES folders(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    method            TEXT NOT NULL,
    url_template      TEXT NOT NULL,
    headers_json      TEXT NOT NULL DEFAULT '[]',
    query_json        TEXT NOT NULL DEFAULT '[]',
    body              TEXT NOT NULL DEFAULT '',
    body_type         TEXT NOT NULL DEFAULT 'none',
    auth_json         TEXT NOT NULL DEFAULT '{}',
    timeout_ms        INTEGER NOT NULL DEFAULT 30000,
    follow_redirects  INTEGER NOT NULL DEFAULT 1,
    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
CREATE INDEX idx_requests_collection ON requests(collection_id);
CREATE INDEX idx_requests_parent ON requests(parent_id);

CREATE TABLE environments (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    variables_json  TEXT NOT NULL DEFAULT '[]',
    protected       INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE TABLE assertions (
    id             TEXT PRIMARY KEY,
    request_id     TEXT NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL,
    path           TEXT NOT NULL DEFAULT '',
    operator       TEXT NOT NULL DEFAULT '',
    expected_json  TEXT,
    options_json   TEXT NOT NULL DEFAULT '{}',
    enabled        INTEGER NOT NULL DEFAULT 1,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL
);
CREATE INDEX idx_assertions_request ON assertions(request_id);

CREATE TABLE baselines (
    id                   TEXT PRIMARY KEY,
    request_id           TEXT NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    environment_id       TEXT REFERENCES environments(id) ON DELETE CASCADE,
    response_snapshot    TEXT NOT NULL,
    selected_paths_json  TEXT NOT NULL DEFAULT '[]',
    ignored_paths_json   TEXT NOT NULL DEFAULT '[]',
    tolerance            REAL NOT NULL DEFAULT 0,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_baselines_request_env ON baselines(request_id, IFNULL(environment_id, ''));

-- History keeps names alongside ids so it stays readable after the
-- collection or request is deleted.
CREATE TABLE runs (
    id                TEXT PRIMARY KEY,
    collection_id     TEXT REFERENCES collections(id) ON DELETE SET NULL,
    collection_name   TEXT NOT NULL DEFAULT '',
    environment_name  TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL,
    started_at        TEXT NOT NULL,
    finished_at       TEXT,
    passed_count      INTEGER NOT NULL DEFAULT 0,
    failed_count      INTEGER NOT NULL DEFAULT 0,
    error_count       INTEGER NOT NULL DEFAULT 0,
    duration_ms       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_runs_started ON runs(started_at DESC);
CREATE INDEX idx_runs_collection ON runs(collection_id, started_at DESC);

CREATE TABLE run_results (
    id                      TEXT PRIMARY KEY,
    run_id                  TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    request_id              TEXT REFERENCES requests(id) ON DELETE SET NULL,
    request_summary_json    TEXT NOT NULL,
    response_summary_json   TEXT,
    assertion_results_json  TEXT NOT NULL DEFAULT '[]',
    regression_json         TEXT,
    error                   TEXT NOT NULL DEFAULT '',
    duration_ms             INTEGER NOT NULL DEFAULT 0,
    status                  TEXT NOT NULL,
    sort_order              INTEGER NOT NULL DEFAULT 0,
    created_at              TEXT NOT NULL
);
CREATE INDEX idx_run_results_run ON run_results(run_id);
CREATE INDEX idx_run_results_request ON run_results(request_id, created_at DESC);

CREATE TABLE settings (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL
);
