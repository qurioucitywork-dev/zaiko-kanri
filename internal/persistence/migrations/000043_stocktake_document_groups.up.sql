ALTER TABLE stocktake_lines ADD COLUMN IF NOT EXISTS document_type TEXT NOT NULL DEFAULT '';
ALTER TABLE stocktake_lines ADD COLUMN IF NOT EXISTS document_id TEXT NOT NULL DEFAULT '';
ALTER TABLE stocktake_lines ADD COLUMN IF NOT EXISTS document_number TEXT NOT NULL DEFAULT '';
ALTER TABLE stocktake_lines ADD COLUMN IF NOT EXISTS document_partner_name TEXT NOT NULL DEFAULT '';
ALTER TABLE stocktake_lines ADD COLUMN IF NOT EXISTS document_checked_at TIMESTAMPTZ;
ALTER TABLE stocktake_lines ADD COLUMN IF NOT EXISTS document_checked_by TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_stocktake_lines_document
    ON stocktake_lines (session_id, document_type, document_id);
