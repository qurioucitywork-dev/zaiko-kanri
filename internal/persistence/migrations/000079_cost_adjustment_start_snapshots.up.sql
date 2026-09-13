CREATE TABLE cost_adjustment_start_snapshots (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    operation_id TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('breakdown', 'combine', 'adjustment')),
    product_id TEXT NOT NULL REFERENCES products(id),
    previous_inventory_status TEXT NOT NULL,
    previous_base_sale_price_minor BIGINT NOT NULL,
    previous_base_sale_currency TEXT NOT NULL,
    previous_internal_comment TEXT NOT NULL,
    previous_cost_adjustment_started_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'confirmed', 'reset')),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    completed_by TEXT REFERENCES users(id),
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX cost_adjustment_start_snapshots_one_active_product
    ON cost_adjustment_start_snapshots(organization_id, product_id)
    WHERE status = 'active';

CREATE INDEX cost_adjustment_start_snapshots_operation
    ON cost_adjustment_start_snapshots(organization_id, operation_id, status);

ALTER TABLE product_cost_merges DROP CONSTRAINT product_cost_merges_status_check;
ALTER TABLE product_cost_merges ADD CONSTRAINT product_cost_merges_status_check
    CHECK (status IN ('draft', 'confirmed', 'cancelled'));
