ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE runs
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

DROP INDEX IF EXISTS idx_sessions_external_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_external_id
    ON sessions (source_type, external_id)
    WHERE external_id IS NOT NULL AND deleted_at IS NULL;

DROP INDEX IF EXISTS idx_runs_external_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_external_id
    ON runs (source_type, external_id)
    WHERE external_id IS NOT NULL AND deleted_at IS NULL;
