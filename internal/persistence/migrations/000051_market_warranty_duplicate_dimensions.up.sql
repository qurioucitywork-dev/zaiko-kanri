ALTER TABLE market_price_records
    ADD COLUMN IF NOT EXISTS warranty_year_month TEXT NOT NULL DEFAULT '';

ALTER TABLE market_import_rows
    ADD COLUMN IF NOT EXISTS warranty_year_month TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN market_price_records.warranty_year_month IS
    'Warranty year and month used as one dimension of market-price duplicate detection (YYYY-MM).';
COMMENT ON COLUMN market_import_rows.warranty_year_month IS
    'Warranty year and month supplied by the market-table CSV (YYYY-MM).';
