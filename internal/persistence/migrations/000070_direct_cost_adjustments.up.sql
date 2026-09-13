ALTER TABLE cost_adjustments
    DROP CONSTRAINT IF EXISTS cost_adjustments_organization_id_source_product_id_key,
    DROP CONSTRAINT IF EXISTS cost_adjustments_adjustment_type_check;

ALTER TABLE cost_adjustments
    ADD CONSTRAINT cost_adjustments_adjustment_type_check CHECK (
        adjustment_type IN ('breakdown','disassemble','combine','swap','direct_increase','direct_decrease')
    );

CREATE TABLE IF NOT EXISTS direct_cost_adjustment_lines (
    id TEXT PRIMARY KEY,
    cost_adjustment_id TEXT NOT NULL REFERENCES cost_adjustments(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position BETWEEN 1 AND 100),
    product_id TEXT NOT NULL REFERENCES products(id),
    product_code TEXT NOT NULL,
    previous_cost_jpy_minor BIGINT NOT NULL CHECK (previous_cost_jpy_minor >= 0),
    adjustment_amount_jpy_minor BIGINT NOT NULL CHECK (adjustment_amount_jpy_minor > 0),
    adjusted_cost_jpy_minor BIGINT NOT NULL CHECK (adjusted_cost_jpy_minor >= 0),
    direction TEXT NOT NULL CHECK (direction IN ('increase','decrease')),
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (cost_adjustment_id, position),
    UNIQUE (cost_adjustment_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_direct_cost_adjustment_lines_product
    ON direct_cost_adjustment_lines (product_id, created_at DESC);

