DROP INDEX IF EXISTS idx_market_price_records_supplier;
DROP TABLE IF EXISTS market_price_accessories;
ALTER TABLE market_price_records
    DROP COLUMN IF EXISTS box_id,
    DROP COLUMN IF EXISTS status_text,
    DROP COLUMN IF EXISTS purchase_date,
    DROP COLUMN IF EXISTS movement_id,
    DROP COLUMN IF EXISTS material_id,
    DROP COLUMN IF EXISTS purchase_staff_profile_id,
    DROP COLUMN IF EXISTS supplier_role_id,
    DROP COLUMN IF EXISTS sku,
    DROP COLUMN IF EXISTS serial_number;
