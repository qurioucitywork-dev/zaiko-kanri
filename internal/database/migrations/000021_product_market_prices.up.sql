CREATE TABLE IF NOT EXISTS product_market_prices (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    purchase_market_price_minor INTEGER NOT NULL DEFAULT 0 CHECK (purchase_market_price_minor >= 0),
    sale_market_price_minor INTEGER NOT NULL DEFAULT 0 CHECK (sale_market_price_minor >= 0),
    updated_by TEXT NOT NULL REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_product_market_prices_product
    ON product_market_prices (organization_id, product_id);
