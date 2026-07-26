CREATE TABLE IF NOT EXISTS sales_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    slip_number TEXT NOT NULL,
    sales_date TEXT NOT NULL,
    customer_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'pending_approval', 'confirmed', 'cancelled')),
    notes TEXT NOT NULL DEFAULT '',
    confirmed_at TEXT,
    confirmed_by TEXT REFERENCES users(id),
    cancelled_at TEXT,
    cancelled_by TEXT REFERENCES users(id),
    cancel_reason TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, slip_number)
);

CREATE TABLE IF NOT EXISTS sales_lines (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    sales_slip_id TEXT NOT NULL REFERENCES sales_slips(id),
    line_number INTEGER NOT NULL,
    product_id TEXT NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_minor INTEGER NOT NULL CHECK (unit_price_minor >= 0),
    sale_currency TEXT NOT NULL CHECK (sale_currency IN ('JPY', 'USD')),
    exchange_rate_snapshot_id TEXT REFERENCES exchange_rate_snapshots(id),
    exchange_rate_scaled INTEGER NOT NULL DEFAULT 0,
    exchange_rate_scale INTEGER NOT NULL DEFAULT 100000000,
    exchange_rate_observed_at TEXT,
    converted_unit_price_jpy INTEGER NOT NULL DEFAULT 0,
    converted_total_jpy INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE (sales_slip_id, line_number),
    UNIQUE (sales_slip_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_sales_slips_org_date
    ON sales_slips (organization_id, sales_date DESC);
CREATE INDEX IF NOT EXISTS idx_sales_lines_product
    ON sales_lines (organization_id, product_id);

CREATE TABLE IF NOT EXISTS shipment_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    shipment_number TEXT NOT NULL,
    shipment_date TEXT NOT NULL,
    recipient_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'confirmed', 'cancelled')),
    notes TEXT NOT NULL DEFAULT '',
    confirmed_at TEXT,
    confirmed_by TEXT REFERENCES users(id),
    cancelled_at TEXT,
    cancelled_by TEXT REFERENCES users(id),
    cancel_reason TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, shipment_number)
);

CREATE TABLE IF NOT EXISTS shipment_lines (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    shipment_slip_id TEXT NOT NULL REFERENCES shipment_slips(id),
    line_number INTEGER NOT NULL,
    product_id TEXT NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    created_at TEXT NOT NULL,
    UNIQUE (shipment_slip_id, line_number),
    UNIQUE (shipment_slip_id, product_id)
);

CREATE TABLE IF NOT EXISTS sales_shipment_allocations (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    sales_line_id TEXT NOT NULL REFERENCES sales_lines(id),
    shipment_line_id TEXT NOT NULL REFERENCES shipment_lines(id),
    allocated_quantity INTEGER NOT NULL CHECK (allocated_quantity > 0),
    created_at TEXT NOT NULL,
    UNIQUE (sales_line_id, shipment_line_id)
);

CREATE INDEX IF NOT EXISTS idx_shipments_org_date
    ON shipment_slips (organization_id, shipment_date DESC);
CREATE INDEX IF NOT EXISTS idx_shipment_lines_product
    ON shipment_lines (organization_id, product_id);
CREATE INDEX IF NOT EXISTS idx_allocations_sales_line
    ON sales_shipment_allocations (organization_id, sales_line_id);
