DROP INDEX IF EXISTS idx_market_price_records_auction;

ALTER TABLE market_import_rows
    DROP COLUMN IF EXISTS auction_code,
    DROP COLUMN IF EXISTS accessory_text,
    DROP COLUMN IF EXISTS movement_text,
    DROP COLUMN IF EXISTS material_text,
    DROP COLUMN IF EXISTS condition_text,
    DROP COLUMN IF EXISTS brand_text;

ALTER TABLE market_price_records
    DROP COLUMN IF EXISTS accessory_text,
    DROP COLUMN IF EXISTS movement_text,
    DROP COLUMN IF EXISTS material_text,
    DROP COLUMN IF EXISTS condition_text,
    DROP COLUMN IF EXISTS auction_house_id;

DROP TABLE IF EXISTS auction_houses;
