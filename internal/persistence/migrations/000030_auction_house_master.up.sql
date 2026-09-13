CREATE TABLE IF NOT EXISTS auction_houses (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_by TEXT REFERENCES users(id),
    updated_by TEXT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, code)
);

CREATE INDEX IF NOT EXISTS idx_auction_houses_org_active
    ON auction_houses (organization_id, is_active, sort_order, code);

ALTER TABLE market_price_records
    ADD COLUMN IF NOT EXISTS auction_house_id TEXT REFERENCES auction_houses(id),
    ADD COLUMN IF NOT EXISTS condition_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS material_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS movement_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS accessory_text TEXT NOT NULL DEFAULT '';

ALTER TABLE market_import_rows
    ADD COLUMN IF NOT EXISTS brand_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS condition_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS material_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS movement_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS accessory_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS auction_code TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_market_price_records_auction
    ON market_price_records (organization_id, auction_house_id, import_date DESC);

COMMENT ON COLUMN market_import_rows.auction_code IS
    'Required auction-house master code supplied by the market-table CSV.';
COMMENT ON COLUMN market_import_rows.brand_text IS
    'Free-text brand supplied by the market-table CSV.';
