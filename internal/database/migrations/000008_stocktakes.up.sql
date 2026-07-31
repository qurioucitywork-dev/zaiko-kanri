CREATE TABLE IF NOT EXISTS stocktakes (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    stocktake_number TEXT NOT NULL,
    stocktake_date TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'completed')),
    expected_count INTEGER NOT NULL DEFAULT 0,
    notes TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_by TEXT REFERENCES users(id),
    completed_at TEXT,
    UNIQUE (organization_id, stocktake_number)
);

CREATE INDEX IF NOT EXISTS idx_stocktakes_org_date
    ON stocktakes (organization_id, stocktake_date DESC, created_at DESC);

CREATE TABLE IF NOT EXISTS stocktake_lines (
    id TEXT PRIMARY KEY,
    stocktake_id TEXT NOT NULL REFERENCES stocktakes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    expected_present INTEGER NOT NULL DEFAULT 1 CHECK (expected_present IN (0, 1)),
    counted_present INTEGER CHECK (counted_present IN (0, 1)),
    notes TEXT NOT NULL DEFAULT '',
    counted_by TEXT REFERENCES users(id),
    counted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (stocktake_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_stocktake_lines_stocktake
    ON stocktake_lines (stocktake_id, counted_present, product_id);
