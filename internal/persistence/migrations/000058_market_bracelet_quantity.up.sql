ALTER TABLE market_price_records
    ADD COLUMN IF NOT EXISTS bracelet_quantity INTEGER
        CHECK (bracelet_quantity IS NULL OR bracelet_quantity >= 0);

ALTER TABLE market_import_rows
    ADD COLUMN IF NOT EXISTS bracelet_quantity INTEGER
        CHECK (bracelet_quantity IS NULL OR bracelet_quantity >= 0);

COMMENT ON COLUMN market_price_records.bracelet_quantity IS
    'BRACELET PARTS selected: number of bracelet links recorded during market research';
COMMENT ON COLUMN market_import_rows.bracelet_quantity IS
    'Preview value for BRACELET PARTS link quantity';
