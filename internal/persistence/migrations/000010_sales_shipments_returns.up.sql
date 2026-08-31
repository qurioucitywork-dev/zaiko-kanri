CREATE TABLE IF NOT EXISTS sales_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    slip_number TEXT NOT NULL,
    buyer_role_id TEXT NOT NULL REFERENCES partner_roles(id),
    sale_date DATE NOT NULL,
    display_currency TEXT NOT NULL CHECK (display_currency IN ('JPY', 'USD')),
    tax_mode TEXT NOT NULL CHECK (tax_mode IN ('taxable', 'tax_exempt')),
    tax_rate_basis_points INTEGER NOT NULL DEFAULT 1000 CHECK (tax_rate_basis_points BETWEEN 0 AND 10000),
    status TEXT NOT NULL CHECK (status IN ('draft', 'pending_approval', 'confirmed', 'cancelled')),
    notes TEXT NOT NULL DEFAULT '',
    confirmed_at TIMESTAMPTZ,
    confirmed_by TEXT REFERENCES users(id),
    cancelled_at TIMESTAMPTZ,
    cancelled_by TEXT REFERENCES users(id),
    cancel_reason TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, slip_number)
);

CREATE INDEX IF NOT EXISTS idx_sales_slips_org_date
    ON sales_slips (organization_id, sale_date DESC, status);

CREATE TABLE IF NOT EXISTS sales_lines (
    id TEXT PRIMARY KEY,
    sales_slip_id TEXT NOT NULL REFERENCES sales_slips(id),
    line_number INTEGER NOT NULL CHECK (line_number > 0),
    product_id TEXT NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL DEFAULT 1 CHECK (quantity = 1),
    unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor >= 0),
    sale_currency TEXT NOT NULL CHECK (sale_currency IN ('JPY', 'USD')),
    subtotal_minor BIGINT NOT NULL CHECK (subtotal_minor >= 0),
    tax_amount_minor BIGINT NOT NULL DEFAULT 0 CHECK (tax_amount_minor >= 0),
    total_minor BIGINT NOT NULL CHECK (total_minor >= 0),
    converted_total_jpy BIGINT NOT NULL CHECK (converted_total_jpy >= 0),
    fx_rate_snapshot_id TEXT REFERENCES exchange_rate_snapshots(id),
    fx_rate_scaled BIGINT NOT NULL DEFAULT 100000000 CHECK (fx_rate_scaled > 0),
    fx_scale BIGINT NOT NULL DEFAULT 100000000 CHECK (fx_scale > 0),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (sales_slip_id, line_number),
    UNIQUE (sales_slip_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_sales_lines_product
    ON sales_lines (product_id, sales_slip_id);

CREATE TABLE IF NOT EXISTS shipment_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    slip_number TEXT NOT NULL,
    buyer_role_id TEXT NOT NULL REFERENCES partner_roles(id),
    sales_slip_id TEXT REFERENCES sales_slips(id),
    shipment_date DATE NOT NULL,
    recipient_name TEXT NOT NULL DEFAULT '',
    recipient_address TEXT NOT NULL DEFAULT '',
    carrier TEXT NOT NULL DEFAULT '',
    tracking_number TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('draft', 'confirmed', 'cancelled')),
    notes TEXT NOT NULL DEFAULT '',
    confirmed_at TIMESTAMPTZ,
    confirmed_by TEXT REFERENCES users(id),
    cancelled_at TIMESTAMPTZ,
    cancelled_by TEXT REFERENCES users(id),
    cancel_reason TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, slip_number)
);

CREATE INDEX IF NOT EXISTS idx_shipment_slips_org_date
    ON shipment_slips (organization_id, shipment_date DESC, status);

CREATE TABLE IF NOT EXISTS shipment_lines (
    id TEXT PRIMARY KEY,
    shipment_slip_id TEXT NOT NULL REFERENCES shipment_slips(id),
    line_number INTEGER NOT NULL CHECK (line_number > 0),
    product_id TEXT NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL DEFAULT 1 CHECK (quantity = 1),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (shipment_slip_id, line_number),
    UNIQUE (shipment_slip_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_shipment_lines_product
    ON shipment_lines (product_id, shipment_slip_id);

CREATE TABLE IF NOT EXISTS return_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    slip_number TEXT NOT NULL,
    operation_type TEXT NOT NULL CHECK (operation_type IN ('return', 'takeout')),
    transaction_date DATE NOT NULL,
    buyer_role_id TEXT REFERENCES partner_roles(id),
    status TEXT NOT NULL CHECK (status IN ('draft', 'confirmed', 'cancelled')),
    reason TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    confirmed_at TIMESTAMPTZ,
    confirmed_by TEXT REFERENCES users(id),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, slip_number)
);

CREATE TABLE IF NOT EXISTS return_lines (
    id TEXT PRIMARY KEY,
    return_slip_id TEXT NOT NULL REFERENCES return_slips(id),
    line_number INTEGER NOT NULL CHECK (line_number > 0),
    product_id TEXT NOT NULL REFERENCES products(id),
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (return_slip_id, line_number),
    UNIQUE (return_slip_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_return_slips_org_date
    ON return_slips (organization_id, transaction_date DESC, status);

ALTER TABLE purchase_slip_lines ADD COLUMN IF NOT EXISTS converted_total_jpy BIGINT;
ALTER TABLE purchase_slip_lines ADD COLUMN IF NOT EXISTS fx_rate_snapshot_id TEXT REFERENCES exchange_rate_snapshots(id);
ALTER TABLE purchase_slip_lines ADD COLUMN IF NOT EXISTS fx_rate_scaled BIGINT;
ALTER TABLE purchase_slip_lines ADD COLUMN IF NOT EXISTS fx_scale BIGINT;

UPDATE purchase_slip_lines
SET converted_total_jpy = unit_cost_minor * quantity
WHERE converted_total_jpy IS NULL AND cost_currency = 'JPY';
