-- SQL helpers for a production PostgreSQL deployment of the metadata service.
CREATE TABLE IF NOT EXISTS metadata (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    version BIGINT NOT NULL,
    attributes JSONB DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_metadata_prefix ON metadata (key text_pattern_ops);

-- Prepared statement names used by the Go service when running against a real PG backend.
PREPARE metadata_upsert (TEXT, JSONB, JSONB, BIGINT) AS
INSERT INTO metadata(key, value, attributes, version)
VALUES ($1, $2, COALESCE($3, '{}'::jsonb), $4)
ON CONFLICT (key) DO UPDATE
SET value = excluded.value,
    attributes = excluded.attributes,
    version = excluded.version,
    updated_at = NOW();

PREPARE metadata_get (TEXT) AS
SELECT key, value, attributes, version FROM metadata WHERE key = $1;

PREPARE metadata_delete (TEXT) AS
DELETE FROM metadata WHERE key = $1;

PREPARE metadata_list (TEXT, INT) AS
SELECT key, value, attributes, version
FROM metadata
WHERE key LIKE $1 || '%'
ORDER BY key ASC
LIMIT $2;
