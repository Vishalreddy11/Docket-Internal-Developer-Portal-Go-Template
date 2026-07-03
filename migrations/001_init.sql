-- Docket Postgres schema.
-- The app also runs this idempotently at startup (see internal/records/postgres.go),
-- so this file is for human reference and for tools like golang-migrate.

CREATE TABLE IF NOT EXISTS file_records (
    id          TEXT PRIMARY KEY,
    file_id     TEXT NOT NULL,
    owner       TEXT NOT NULL,
    file_name   TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    action      TEXT NOT NULL,  -- 'upload' | 'view' | 'delete'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_file_records_file_id    ON file_records(file_id);
CREATE INDEX IF NOT EXISTS idx_file_records_created_at ON file_records(created_at DESC);
