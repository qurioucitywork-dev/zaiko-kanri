ALTER TABLE stocktake_sessions ADD COLUMN IF NOT EXISTS inventory_date DATE;
ALTER TABLE stocktake_lines ADD COLUMN IF NOT EXISTS shipment_issued_at TIMESTAMPTZ;
ALTER TABLE stocktake_lines ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;
