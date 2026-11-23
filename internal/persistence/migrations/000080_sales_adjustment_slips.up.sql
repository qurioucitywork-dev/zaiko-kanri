CREATE TABLE IF NOT EXISTS sales_adjustment_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    slip_number TEXT NOT NULL,
    source_sales_slip_id TEXT NOT NULL REFERENCES sales_slips(id),
    adjustment_date DATE NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('increase','decrease')),
    display_currency TEXT NOT NULL CHECK (display_currency IN ('JPY','USD','EUR','HKD')),
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('confirmed','cancelled')),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, slip_number)
);

CREATE TABLE IF NOT EXISTS sales_adjustment_lines (
    id TEXT PRIMARY KEY,
    sales_adjustment_slip_id TEXT NOT NULL REFERENCES sales_adjustment_slips(id) ON DELETE CASCADE,
    source_sales_line_id TEXT NOT NULL REFERENCES sales_lines(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    line_number INTEGER NOT NULL CHECK (line_number > 0),
    adjustment_amount_minor BIGINT NOT NULL CHECK (adjustment_amount_minor > 0),
    converted_adjustment_jpy BIGINT NOT NULL CHECK (converted_adjustment_jpy > 0),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (sales_adjustment_slip_id, source_sales_line_id)
);

CREATE INDEX IF NOT EXISTS idx_sales_adjustments_source ON sales_adjustment_slips (source_sales_slip_id, adjustment_date DESC);
CREATE INDEX IF NOT EXISTS idx_sales_adjustment_lines_product ON sales_adjustment_lines (product_id, sales_adjustment_slip_id);
