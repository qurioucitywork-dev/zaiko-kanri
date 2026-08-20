DROP INDEX IF EXISTS idx_stocktake_lines_document;
ALTER TABLE stocktake_lines DROP COLUMN IF EXISTS document_checked_by;
ALTER TABLE stocktake_lines DROP COLUMN IF EXISTS document_checked_at;
ALTER TABLE stocktake_lines DROP COLUMN IF EXISTS document_partner_name;
ALTER TABLE stocktake_lines DROP COLUMN IF EXISTS document_number;
ALTER TABLE stocktake_lines DROP COLUMN IF EXISTS document_id;
ALTER TABLE stocktake_lines DROP COLUMN IF EXISTS document_type;
