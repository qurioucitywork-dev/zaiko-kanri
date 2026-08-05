CREATE TABLE IF NOT EXISTS purchase_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    slip_number TEXT NOT NULL,
    supplier_role_id TEXT NOT NULL REFERENCES partner_roles(id),
    purchase_staff_profile_id TEXT REFERENCES staff_profiles(id),
    purchase_date DATE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'pending_approval', 'confirmed', 'cancelled')),
    is_simple BOOLEAN NOT NULL DEFAULT FALSE,
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

CREATE INDEX IF NOT EXISTS idx_purchase_slips_org_date
    ON purchase_slips (organization_id, purchase_date DESC);

CREATE TABLE IF NOT EXISTS purchase_slip_lines (
    id TEXT PRIMARY KEY,
    purchase_slip_id TEXT NOT NULL REFERENCES purchase_slips(id),
    line_number INTEGER NOT NULL CHECK (line_number > 0),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_cost_minor BIGINT NOT NULL CHECK (unit_cost_minor >= 0),
    cost_currency TEXT NOT NULL CHECK (cost_currency IN ('JPY', 'USD')),
    base_sale_price_minor BIGINT NOT NULL DEFAULT 0 CHECK (base_sale_price_minor >= 0),
    base_sale_currency TEXT NOT NULL DEFAULT 'USD' CHECK (base_sale_currency IN ('JPY', 'USD')),
    brand_id TEXT REFERENCES brands(id),
    material_id TEXT REFERENCES materials(id),
    movement_id TEXT REFERENCES movements(id),
    condition_id TEXT REFERENCES product_conditions(id),
    brand_text TEXT NOT NULL DEFAULT '',
    model_number TEXT NOT NULL DEFAULT '',
    reference_number TEXT NOT NULL DEFAULT '',
    serial_number TEXT NOT NULL DEFAULT '',
    product_type TEXT NOT NULL DEFAULT '腕時計',
    sku TEXT NOT NULL DEFAULT '',
    generated_product_count INTEGER NOT NULL DEFAULT 0 CHECK (generated_product_count >= 0),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (purchase_slip_id, line_number)
);

CREATE TABLE IF NOT EXISTS product_code_sequences (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    business_date DATE NOT NULL,
    last_sequence INTEGER NOT NULL CHECK (last_sequence BETWEEN 0 AND 999),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (organization_id, business_date)
);

CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_code TEXT NOT NULL,
    sku TEXT NOT NULL DEFAULT '',
    brand TEXT NOT NULL DEFAULT '',
    brand_id TEXT REFERENCES brands(id),
    model_number TEXT NOT NULL DEFAULT '',
    reference_number TEXT NOT NULL DEFAULT '',
    serial_number TEXT NOT NULL DEFAULT '',
    product_type TEXT NOT NULL DEFAULT '腕時計',
    material_id TEXT REFERENCES materials(id),
    movement_id TEXT REFERENCES movements(id),
    condition_id TEXT REFERENCES product_conditions(id),
    supplier_id TEXT NOT NULL DEFAULT '',
    supplier_role_id TEXT REFERENCES partner_roles(id),
    purchase_staff_profile_id TEXT REFERENCES staff_profiles(id),
    purchase_slip_line_id TEXT REFERENCES purchase_slip_lines(id),
    purchase_date DATE,
    cost_amount_minor BIGINT NOT NULL DEFAULT 0 CHECK (cost_amount_minor >= 0),
    cost_currency TEXT NOT NULL DEFAULT 'JPY' CHECK (cost_currency IN ('JPY', 'USD')),
    base_sale_price_minor BIGINT NOT NULL DEFAULT 0 CHECK (base_sale_price_minor >= 0),
    base_sale_currency TEXT NOT NULL DEFAULT 'USD' CHECK (base_sale_currency IN ('JPY', 'USD')),
    inventory_status TEXT NOT NULL DEFAULT 'in_stock' CHECK (inventory_status IN ('purchasing', 'in_stock', 'reserved', 'sold', 'shipped', 'cancelled', 'invalid')),
    publication_status TEXT NOT NULL DEFAULT 'private' CHECK (publication_status IN ('private', 'public')),
    condition_text TEXT NOT NULL DEFAULT '',
    accessories TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    cancelled_at TIMESTAMPTZ,
    cancelled_by TEXT REFERENCES users(id),
    cancel_reason TEXT NOT NULL DEFAULT '',
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, product_code)
);

ALTER TABLE products ADD COLUMN IF NOT EXISTS brand_id TEXT REFERENCES brands(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS material_id TEXT REFERENCES materials(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS movement_id TEXT REFERENCES movements(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS condition_id TEXT REFERENCES product_conditions(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS reference_number TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN IF NOT EXISTS supplier_role_id TEXT REFERENCES partner_roles(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS purchase_staff_profile_id TEXT REFERENCES staff_profiles(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS purchase_slip_line_id TEXT REFERENCES purchase_slip_lines(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;
ALTER TABLE products ADD COLUMN IF NOT EXISTS cancelled_by TEXT REFERENCES users(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS cancel_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
ALTER TABLE products ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;

ALTER TABLE products
    ALTER COLUMN purchase_date TYPE DATE
    USING CASE WHEN purchase_date IS NULL OR purchase_date::text = '' THEN NULL ELSE purchase_date::text::date END;
ALTER TABLE products
    ALTER COLUMN deleted_at TYPE TIMESTAMPTZ
    USING CASE WHEN deleted_at IS NULL OR deleted_at::text = '' THEN NULL ELSE deleted_at::text::timestamptz END;

UPDATE products SET
    sku = COALESCE(sku, ''),
    brand = COALESCE(brand, ''),
    model_number = COALESCE(model_number, ''),
    serial_number = COALESCE(serial_number, ''),
    product_type = COALESCE(product_type, '腕時計'),
    supplier_id = COALESCE(supplier_id, ''),
    cost_amount_minor = COALESCE(cost_amount_minor, 0),
    cost_currency = COALESCE(cost_currency, 'JPY'),
    base_sale_price_minor = COALESCE(base_sale_price_minor, 0),
    base_sale_currency = COALESCE(base_sale_currency, 'USD'),
    inventory_status = COALESCE(inventory_status, 'in_stock'),
    publication_status = COALESCE(publication_status, 'private'),
    condition_text = COALESCE(condition_text, ''),
    accessories = COALESCE(accessories, ''),
    created_at = COALESCE(created_at, CURRENT_TIMESTAMP),
    updated_at = COALESCE(updated_at, CURRENT_TIMESTAMP);

ALTER TABLE products ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE products ALTER COLUMN product_code SET NOT NULL;
ALTER TABLE products ALTER COLUMN sku SET NOT NULL;
ALTER TABLE products ALTER COLUMN brand SET NOT NULL;
ALTER TABLE products ALTER COLUMN model_number SET NOT NULL;
ALTER TABLE products ALTER COLUMN serial_number SET NOT NULL;
ALTER TABLE products ALTER COLUMN product_type SET NOT NULL;
ALTER TABLE products ALTER COLUMN supplier_id SET NOT NULL;
ALTER TABLE products ALTER COLUMN cost_amount_minor SET NOT NULL;
ALTER TABLE products ALTER COLUMN cost_currency SET NOT NULL;
ALTER TABLE products ALTER COLUMN base_sale_price_minor SET NOT NULL;
ALTER TABLE products ALTER COLUMN base_sale_currency SET NOT NULL;
ALTER TABLE products ALTER COLUMN inventory_status SET NOT NULL;
ALTER TABLE products ALTER COLUMN publication_status SET NOT NULL;
ALTER TABLE products ALTER COLUMN condition_text SET NOT NULL;
ALTER TABLE products ALTER COLUMN accessories SET NOT NULL;
ALTER TABLE products ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE products ALTER COLUMN updated_at SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_products_org_code ON products (organization_id, product_code);
CREATE INDEX IF NOT EXISTS idx_products_org_status ON products (organization_id, inventory_status, purchase_date DESC);
CREATE INDEX IF NOT EXISTS idx_products_org_serial ON products (organization_id, serial_number);
CREATE INDEX IF NOT EXISTS idx_products_org_sku ON products (organization_id, sku);

CREATE TABLE IF NOT EXISTS product_accessories (
    product_id TEXT NOT NULL REFERENCES products(id),
    accessory_id TEXT NOT NULL REFERENCES accessories(id),
    quantity INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0),
    PRIMARY KEY (product_id, accessory_id)
);

CREATE TABLE IF NOT EXISTS product_files (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    storage_driver TEXT NOT NULL CHECK (storage_driver IN ('local', 's3')),
    object_key TEXT NOT NULL,
    original_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    sha256 TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    uploaded_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, object_key)
);

CREATE INDEX IF NOT EXISTS idx_product_files_product
    ON product_files (organization_id, product_id, sort_order);

CREATE TABLE IF NOT EXISTS inventory_events (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    event_type TEXT NOT NULL,
    from_status TEXT NOT NULL DEFAULT '',
    to_status TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    actor_user_id TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_inventory_events_product
    ON inventory_events (organization_id, product_id, created_at DESC);

CREATE TABLE IF NOT EXISTS exchange_rate_snapshots (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    base_currency TEXT NOT NULL CHECK (base_currency IN ('USD', 'JPY')),
    quote_currency TEXT NOT NULL CHECK (quote_currency IN ('USD', 'JPY')),
    rate_scaled BIGINT NOT NULL CHECK (rate_scaled > 0),
    scale BIGINT NOT NULL DEFAULT 100000000 CHECK (scale > 0),
    provider TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (base_currency <> quote_currency)
);

CREATE INDEX IF NOT EXISTS idx_exchange_rates_latest
    ON exchange_rate_snapshots (organization_id, base_currency, quote_currency, observed_at DESC);

CREATE TABLE IF NOT EXISTS market_import_batches (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    file_name TEXT NOT NULL,
    source_object_key TEXT NOT NULL DEFAULT '',
    source_sha256 TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('previewed', 'pending_approval', 'committed', 'rejected')),
    total_rows INTEGER NOT NULL DEFAULT 0,
    valid_rows INTEGER NOT NULL DEFAULT 0,
    error_rows INTEGER NOT NULL DEFAULT 0,
    duplicate_rows INTEGER NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    committed_by TEXT REFERENCES users(id),
    committed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS market_price_records (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    import_date DATE NOT NULL,
    brand_id TEXT REFERENCES brands(id),
    brand_text TEXT NOT NULL DEFAULT '',
    model_number TEXT NOT NULL DEFAULT '',
    reference_number TEXT NOT NULL DEFAULT '',
    condition_id TEXT REFERENCES product_conditions(id),
    purchase_price_minor BIGINT NOT NULL DEFAULT 0 CHECK (purchase_price_minor >= 0),
    purchase_currency TEXT NOT NULL DEFAULT 'JPY' CHECK (purchase_currency IN ('JPY', 'USD')),
    market_price_minor BIGINT NOT NULL DEFAULT 0 CHECK (market_price_minor >= 0),
    market_currency TEXT NOT NULL DEFAULT 'USD' CHECK (market_currency IN ('JPY', 'USD')),
    source TEXT NOT NULL DEFAULT 'manual',
    notes TEXT NOT NULL DEFAULT '',
    import_batch_id TEXT REFERENCES market_import_batches(id),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_market_prices_org_date
    ON market_price_records (organization_id, import_date DESC);
CREATE INDEX IF NOT EXISTS idx_market_prices_lookup
    ON market_price_records (organization_id, brand_id, reference_number, market_currency, import_date DESC);

CREATE TABLE IF NOT EXISTS market_import_rows (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL REFERENCES market_import_batches(id),
    row_number INTEGER NOT NULL,
    import_date DATE,
    brand_code TEXT NOT NULL DEFAULT '',
    model_number TEXT NOT NULL DEFAULT '',
    reference_number TEXT NOT NULL DEFAULT '',
    condition_code TEXT NOT NULL DEFAULT '',
    purchase_price_minor BIGINT NOT NULL DEFAULT 0,
    market_price_minor BIGINT NOT NULL DEFAULT 0,
    raw_json JSONB NOT NULL,
    is_valid BOOLEAN NOT NULL DEFAULT FALSE,
    error_message TEXT NOT NULL DEFAULT '',
    duplicate_candidate_id TEXT REFERENCES market_price_records(id),
    UNIQUE (batch_id, row_number)
);

CREATE INDEX IF NOT EXISTS idx_market_import_rows_batch
    ON market_import_rows (batch_id, row_number);
