ALTER TABLE market_import_rows
    DROP COLUMN IF EXISTS warranty_year_month;

ALTER TABLE market_price_records
    DROP COLUMN IF EXISTS warranty_year_month;
