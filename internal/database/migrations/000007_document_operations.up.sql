CREATE TABLE IF NOT EXISTS document_generation_events (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    document_type TEXT NOT NULL,
    document_id TEXT NOT NULL DEFAULT '',
    document_number TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL CHECK (action IN ('preview', 'print', 'download')),
    output_format TEXT NOT NULL CHECK (output_format IN ('html', 'pdf', 'csv')),
    file_name TEXT NOT NULL DEFAULT '',
    storage_driver TEXT NOT NULL DEFAULT 'local' CHECK (storage_driver IN ('local', 's3')),
    object_key TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_document_events_org_created
    ON document_generation_events (organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_document_events_document
    ON document_generation_events (organization_id, document_type, document_id, created_at DESC);
