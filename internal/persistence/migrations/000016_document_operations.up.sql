ALTER TABLE return_slips
    ADD COLUMN IF NOT EXISTS carrier TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tracking_number TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS document_generation_events (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    document_type TEXT NOT NULL,
    document_id TEXT NOT NULL DEFAULT '',
    document_number TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    output_format TEXT NOT NULL,
    file_name TEXT NOT NULL DEFAULT '',
    storage_driver TEXT NOT NULL DEFAULT 'local',
    object_key TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_document_generation_events_document
    ON document_generation_events (organization_id, document_type, document_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_document_generation_events_created
    ON document_generation_events (organization_id, created_at DESC);
