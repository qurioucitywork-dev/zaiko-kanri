ALTER TABLE market_import_rows
    DROP COLUMN IF EXISTS bracelet_quantity;

ALTER TABLE market_price_records
    DROP COLUMN IF EXISTS bracelet_quantity;
