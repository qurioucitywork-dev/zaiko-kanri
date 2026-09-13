ALTER TABLE products
    ADD COLUMN IF NOT EXISTS inventory_valuation_excluded BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS inventory_valuation_exclusion_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS inventory_valuation_excluded_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_products_current_inventory_valuation
    ON products (organization_id, inventory_status)
    WHERE deleted_at IS NULL AND inventory_valuation_excluded = FALSE;

