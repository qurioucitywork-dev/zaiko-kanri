DROP INDEX IF EXISTS idx_products_current_inventory_valuation;

ALTER TABLE products
    DROP COLUMN IF EXISTS inventory_valuation_excluded_at,
    DROP COLUMN IF EXISTS inventory_valuation_exclusion_reason,
    DROP COLUMN IF EXISTS inventory_valuation_excluded;

