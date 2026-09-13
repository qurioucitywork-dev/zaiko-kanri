ALTER TABLE market_import_rows
    ADD COLUMN IF NOT EXISTS purchase_currency TEXT NOT NULL DEFAULT 'JPY'
    CHECK (purchase_currency IN ('JPY', 'USD'));

ALTER TABLE market_import_rows
    ADD COLUMN IF NOT EXISTS market_currency TEXT NOT NULL DEFAULT 'USD'
    CHECK (market_currency IN ('JPY', 'USD'));

ALTER TABLE market_import_rows
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'csv';

ALTER TABLE market_import_rows
    ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';
