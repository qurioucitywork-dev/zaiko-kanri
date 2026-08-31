ALTER TABLE stocktake_lines DROP COLUMN IF EXISTS resolved_at;
ALTER TABLE stocktake_lines DROP COLUMN IF EXISTS shipment_issued_at;
ALTER TABLE stocktake_sessions DROP COLUMN IF EXISTS inventory_date;
