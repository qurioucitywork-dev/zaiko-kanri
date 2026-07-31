CREATE TABLE IF NOT EXISTS master_records (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    category TEXT NOT NULL,
    record_code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, category, record_code)
);

CREATE INDEX IF NOT EXISTS idx_master_records_org_category
    ON master_records (organization_id, category, is_active, record_code);
