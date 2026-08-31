ALTER TABLE market_import_rows
    ADD COLUMN IF NOT EXISTS sku TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS material_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS movement_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS accessory_codes TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN market_import_rows.import_date IS
    'Auction event date supplied by the market-table CSV.';
COMMENT ON COLUMN market_import_rows.source IS
    'Auction name supplied by the market-table CSV.';
COMMENT ON COLUMN market_import_rows.market_price_minor IS
    'Winning bid price. New market-table imports use JPY.';
