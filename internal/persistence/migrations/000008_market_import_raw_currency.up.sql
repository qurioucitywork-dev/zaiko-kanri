ALTER TABLE market_import_rows
    DROP CONSTRAINT IF EXISTS market_import_rows_purchase_currency_check;

ALTER TABLE market_import_rows
    DROP CONSTRAINT IF EXISTS market_import_rows_market_currency_check;
