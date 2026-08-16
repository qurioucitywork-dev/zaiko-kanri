CREATE TABLE IF NOT EXISTS stocktake_sessions (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed')),
    started_by TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    saved_at TIMESTAMPTZ NOT NULL,
    completed_by TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_stocktake_active_organization
    ON stocktake_sessions (organization_id) WHERE status = 'in_progress';

CREATE TABLE IF NOT EXISTS stocktake_lines (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES stocktake_sessions(id) ON DELETE CASCADE,
    product_id TEXT,
    product_code TEXT NOT NULL,
    line_type TEXT NOT NULL CHECK (line_type IN ('expected_missing', 'unknown_inventory')),
    result_status TEXT NOT NULL CHECK (result_status IN ('missing', 'verified', 'unknown')),
    source TEXT NOT NULL DEFAULT 'snapshot',
    inventory_status TEXT NOT NULL DEFAULT '',
    brand TEXT NOT NULL DEFAULT '',
    model_number TEXT NOT NULL DEFAULT '',
    reference_number TEXT NOT NULL DEFAULT '',
    serial_number TEXT NOT NULL DEFAULT '',
    purchase_price_minor BIGINT NOT NULL DEFAULT 0,
    reason TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_stocktake_expected_product
    ON stocktake_lines (session_id, product_id) WHERE line_type = 'expected_missing' AND product_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_stocktake_unknown_code
    ON stocktake_lines (session_id, product_code) WHERE line_type = 'unknown_inventory';
CREATE INDEX IF NOT EXISTS idx_stocktake_lines_session ON stocktake_lines (organization_id, session_id, line_type, result_status);
