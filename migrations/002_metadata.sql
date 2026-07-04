-- File metadata store. JSONB column keeps the schema open-ended so different
-- file types can attach different fields. GIN index
-- on the JSONB enables containment queries like `doc @> '{"owner":"alice"}'`.
-- The app runs this idempotently at startup (see internal/metadata/postgres.go).

CREATE TABLE IF NOT EXISTS file_metadata (
    id           TEXT PRIMARY KEY,
    uploaded_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    doc          JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_file_metadata_uploaded_at ON file_metadata(uploaded_at DESC);
CREATE INDEX IF NOT EXISTS idx_file_metadata_doc_gin     ON file_metadata USING GIN (doc);
