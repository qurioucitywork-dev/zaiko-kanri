CREATE TABLE IF NOT EXISTS document_sequences (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    document_type TEXT NOT NULL,
    business_year INTEGER NOT NULL CHECK (business_year BETWEEN 2000 AND 9999),
    last_sequence INTEGER NOT NULL CHECK (last_sequence > 0),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (organization_id, document_type, business_year)
);

CREATE INDEX IF NOT EXISTS idx_document_sequences_org
    ON document_sequences (organization_id, document_type, business_year);
