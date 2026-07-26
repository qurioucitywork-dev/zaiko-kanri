CREATE TABLE IF NOT EXISTS suppliers (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    supplier_code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, supplier_code)
);

CREATE TABLE IF NOT EXISTS purchase_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    slip_number TEXT NOT NULL,
    supplier_id TEXT NOT NULL REFERENCES suppliers(id),
    purchase_date TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'confirmed', 'cancelled')),
    is_simple INTEGER NOT NULL DEFAULT 0,
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

CREATE INDEX IF NOT EXISTS idx_purchase_slips_org_date
    ON purchase_slips (organization_id, purchase_date DESC);

CREATE TABLE IF NOT EXISTS purchase_slip_lines (
    id TEXT PRIMARY KEY,
    purchase_slip_id TEXT NOT NULL REFERENCES purchase_slips(id),
    line_number INTEGER NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_cost_minor INTEGER NOT NULL CHECK (unit_cost_minor >= 0),
    currency TEXT NOT NULL CHECK (currency IN ('JPY', 'USD')),
    brand TEXT NOT NULL,
    model_number TEXT NOT NULL DEFAULT '',
    product_type TEXT NOT NULL,
    generated_product_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE (purchase_slip_id, line_number)
);

CREATE TABLE IF NOT EXISTS product_code_sequences (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    purchase_date TEXT NOT NULL,
    last_sequence INTEGER NOT NULL CHECK (last_sequence BETWEEN 0 AND 999),
    PRIMARY KEY (organization_id, purchase_date)
);

CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_code TEXT NOT NULL,
    sku TEXT NOT NULL DEFAULT '',
    brand TEXT NOT NULL,
    model_number TEXT NOT NULL DEFAULT '',
    serial_number TEXT NOT NULL DEFAULT '',
    product_type TEXT NOT NULL,
    purchase_slip_line_id TEXT NOT NULL REFERENCES purchase_slip_lines(id),
    supplier_id TEXT NOT NULL REFERENCES suppliers(id),
    purchase_date TEXT NOT NULL,
    cost_amount_minor INTEGER NOT NULL CHECK (cost_amount_minor >= 0),
    cost_currency TEXT NOT NULL CHECK (cost_currency IN ('JPY', 'USD')),
    base_sale_price_minor INTEGER NOT NULL DEFAULT 0 CHECK (base_sale_price_minor >= 0),
    base_sale_currency TEXT NOT NULL DEFAULT 'USD' CHECK (base_sale_currency IN ('JPY', 'USD')),
    inventory_status TEXT NOT NULL CHECK (inventory_status IN ('purchasing', 'in_stock', 'reserved', 'sold', 'shipped', 'cancelled', 'invalid')),
    publication_status TEXT NOT NULL DEFAULT 'private' CHECK (publication_status IN ('private', 'public')),
    condition_text TEXT NOT NULL DEFAULT '',
    accessories TEXT NOT NULL DEFAULT '',
    cancelled_at TEXT,
    cancelled_by TEXT REFERENCES users(id),
    cancel_reason TEXT NOT NULL DEFAULT '',
    deleted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, product_code)
);

CREATE INDEX IF NOT EXISTS idx_products_org_status
    ON products (organization_id, inventory_status, purchase_date DESC);
CREATE INDEX IF NOT EXISTS idx_products_org_serial
    ON products (organization_id, serial_number);
CREATE INDEX IF NOT EXISTS idx_products_org_sku
    ON products (organization_id, sku);

CREATE TABLE IF NOT EXISTS product_images (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    storage_path TEXT NOT NULL,
    original_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes > 0),
    sort_order INTEGER NOT NULL DEFAULT 0,
    uploaded_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_product_images_product
    ON product_images (organization_id, product_id, sort_order);

CREATE TABLE IF NOT EXISTS inventory_events (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    event_type TEXT NOT NULL,
    from_status TEXT NOT NULL DEFAULT '',
    to_status TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    actor_user_id TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_inventory_events_product
    ON inventory_events (organization_id, product_id, created_at DESC);

CREATE TABLE IF NOT EXISTS serial_duplicate_overrides (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    serial_number TEXT NOT NULL,
    candidate_product_ids_json TEXT NOT NULL,
    reason TEXT NOT NULL,
    actor_user_id TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL
);
