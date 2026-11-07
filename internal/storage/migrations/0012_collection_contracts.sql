-- OpenAPI contract per collection (docs/openapi-contract.md): the spec as
-- JSON and the check's options, deleted with the collection.
CREATE TABLE collection_contracts (
    collection_id       TEXT PRIMARY KEY REFERENCES collections(id) ON DELETE CASCADE,
    spec_json           TEXT NOT NULL,
    source_name         TEXT NOT NULL DEFAULT '',
    enabled             INTEGER NOT NULL DEFAULT 1,
    allow_undocumented  INTEGER NOT NULL DEFAULT 0,
    updated_at          TEXT NOT NULL
);
