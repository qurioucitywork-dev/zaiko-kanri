ALTER TABLE market_import_rows
    DROP COLUMN IF EXISTS accessory_codes,
    DROP COLUMN IF EXISTS movement_code,
    DROP COLUMN IF EXISTS material_code,
    DROP COLUMN IF EXISTS sku;
