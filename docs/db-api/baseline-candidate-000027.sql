-- BASELINE CANDIDATE THROUGH LEGACY MIGRATION 000027
-- GENERATED FOR LOCAL/D1 COMPATIBILITY REVIEW ONLY.
-- DO NOT APPLY TO THE CURRENT DATABASE, PRODUCTION, OR ANY REMOTE RESOURCE.
--
-- This candidate preserves the legacy DDL order so every statement remains
-- traceable to 000001-000027. It intentionally omits 000023 because that file
-- contains preview-only data changes and no schema.
--
-- This is not yet a production D1 baseline: it contains ALTER TABLE and the
-- 000020 exchange_rate_snapshots rebuild. See 08-baseline-d1-compatibility.md.
-- A deployable D1 baseline must be flattened after capability verification.

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000001_phase1.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS permissions (
    permission_key TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS roles (
    role_key TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_key TEXT NOT NULL REFERENCES roles(role_key),
    permission_key TEXT NOT NULL REFERENCES permissions(permission_key),
    PRIMARY KEY (role_key, permission_key)
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role_key TEXT NOT NULL REFERENCES roles(role_key),
    is_active INTEGER NOT NULL DEFAULT 1,
    last_login_at TEXT,
    deleted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, username)
);

CREATE INDEX IF NOT EXISTS idx_users_org_active
    ON users (organization_id, is_active);

CREATE TABLE IF NOT EXISTS user_permissions (
    user_id TEXT NOT NULL REFERENCES users(id),
    permission_key TEXT NOT NULL REFERENCES permissions(permission_key),
    effect TEXT NOT NULL CHECK (effect IN ('allow', 'deny')),
    PRIMARY KEY (user_id, permission_key)
);

CREATE TABLE IF NOT EXISTS organization_settings (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    setting_key TEXT NOT NULL,
    setting_value TEXT NOT NULL,
    value_type TEXT NOT NULL DEFAULT 'string',
    is_configured INTEGER NOT NULL DEFAULT 0,
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, setting_key)
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    csrf_token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);

CREATE TABLE IF NOT EXISTS login_csrf_tokens (
    token_hash TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_login_csrf_expiry
    ON login_csrf_tokens (expires_at);

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    organization_id TEXT REFERENCES organizations(id),
    actor_user_id TEXT REFERENCES users(id),
    applicant_user_id TEXT REFERENCES users(id),
    approver_user_id TEXT REFERENCES users(id),
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    action TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '{}',
    after_json TEXT NOT NULL DEFAULT '{}',
    reason TEXT NOT NULL DEFAULT '',
    comment TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL,
    result TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_org_created
    ON audit_logs (organization_id, created_at DESC);

CREATE TRIGGER IF NOT EXISTS audit_logs_no_update
BEFORE UPDATE ON audit_logs
BEGIN
    SELECT RAISE(ABORT, 'audit logs are immutable');
END;

CREATE TRIGGER IF NOT EXISTS audit_logs_no_delete
BEFORE DELETE ON audit_logs
BEGIN
    SELECT RAISE(ABORT, 'audit logs are immutable');
END;

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000002_inventory.up.sql
-- -----------------------------------------------------------------------------
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

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000003_market.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS exchange_rate_snapshots (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    base_currency TEXT NOT NULL CHECK (base_currency IN ('USD', 'JPY')),
    quote_currency TEXT NOT NULL CHECK (quote_currency IN ('USD', 'JPY')),
    rate_scaled INTEGER NOT NULL CHECK (rate_scaled > 0),
    scale INTEGER NOT NULL DEFAULT 100000000,
    provider TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    CHECK (base_currency <> quote_currency)
);

CREATE INDEX IF NOT EXISTS idx_exchange_rates_latest
    ON exchange_rate_snapshots (organization_id, base_currency, quote_currency, observed_at DESC);

CREATE TABLE IF NOT EXISTS market_price_records (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    market_date TEXT NOT NULL,
    brand TEXT NOT NULL,
    model_number TEXT NOT NULL DEFAULT '',
    product_type TEXT NOT NULL,
    price_minor INTEGER NOT NULL CHECK (price_minor >= 0),
    currency TEXT NOT NULL CHECK (currency IN ('JPY', 'USD')),
    source TEXT NOT NULL DEFAULT 'manual',
    notes TEXT NOT NULL DEFAULT '',
    import_batch_id TEXT,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_market_prices_org_date
    ON market_price_records (organization_id, market_date DESC);
CREATE INDEX IF NOT EXISTS idx_market_prices_lookup
    ON market_price_records (organization_id, brand, model_number, currency, market_date DESC);

CREATE TABLE IF NOT EXISTS market_import_batches (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    file_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('previewed', 'pending_approval', 'committed', 'rejected')),
    total_rows INTEGER NOT NULL DEFAULT 0,
    valid_rows INTEGER NOT NULL DEFAULT 0,
    error_rows INTEGER NOT NULL DEFAULT 0,
    duplicate_rows INTEGER NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    committed_by TEXT REFERENCES users(id),
    committed_at TEXT
);

CREATE TABLE IF NOT EXISTS market_import_rows (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL REFERENCES market_import_batches(id),
    row_number INTEGER NOT NULL,
    market_date TEXT NOT NULL DEFAULT '',
    brand TEXT NOT NULL DEFAULT '',
    model_number TEXT NOT NULL DEFAULT '',
    product_type TEXT NOT NULL DEFAULT '',
    price_minor INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    raw_json TEXT NOT NULL,
    is_valid INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    duplicate_candidate_id TEXT REFERENCES market_price_records(id),
    UNIQUE (batch_id, row_number)
);

CREATE INDEX IF NOT EXISTS idx_market_import_rows_batch
    ON market_import_rows (batch_id, row_number);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000004_sales_shipments.up.sql
-- -----------------------------------------------------------------------------
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

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000005_requests_reservations.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS purchase_requests (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    request_number TEXT NOT NULL,
    guest_name TEXT NOT NULL,
    guest_email TEXT NOT NULL,
    guest_phone TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled', 'expired', 'sold')),
    requested_at TEXT NOT NULL,
    reviewed_at TEXT,
    reviewed_by TEXT REFERENCES users(id),
    rejection_reason TEXT NOT NULL DEFAULT '',
    cancelled_at TEXT,
    cancel_reason TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, request_number)
);

CREATE INDEX IF NOT EXISTS idx_purchase_requests_org_status
    ON purchase_requests (organization_id, status, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_purchase_requests_product
    ON purchase_requests (organization_id, product_id, status);

CREATE TABLE IF NOT EXISTS reservations (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    purchase_request_id TEXT NOT NULL REFERENCES purchase_requests(id),
    status TEXT NOT NULL CHECK (status IN ('active', 'expired', 'released', 'fulfilled')),
    starts_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    released_at TEXT,
    release_reason TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (purchase_request_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_active_reservation_product
    ON reservations (organization_id, product_id)
    WHERE status='active';
CREATE INDEX IF NOT EXISTS idx_reservations_expiry
    ON reservations (organization_id, status, expires_at);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000006_approvals.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS approval_requests (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    approval_type TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    action_key TEXT NOT NULL,
    applicant_user_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'returned', 'rejected', 'cancelled', 'expired')),
    requested_snapshot TEXT NOT NULL,
    requested_snapshot_hash TEXT NOT NULL,
    request_reason TEXT NOT NULL DEFAULT '',
    action_payload_json TEXT NOT NULL DEFAULT '{}',
    requested_at TEXT NOT NULL,
    expires_at TEXT,
    decided_at TEXT,
    decided_by TEXT REFERENCES users(id),
    executed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_pending_approval_target
    ON approval_requests (organization_id, target_type, target_id, action_key)
    WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_approvals_org_status
    ON approval_requests (organization_id, status, requested_at DESC);

CREATE TABLE IF NOT EXISTS approval_actions (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    approval_request_id TEXT NOT NULL REFERENCES approval_requests(id),
    actor_user_id TEXT NOT NULL REFERENCES users(id),
    action TEXT NOT NULL CHECK (action IN ('requested', 'approved', 'returned', 'rejected', 'cancelled', 'expired')),
    comment TEXT NOT NULL DEFAULT '',
    acted_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_approval_actions_request
    ON approval_actions (organization_id, approval_request_id, acted_at);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000007_masters.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS master_records (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    category TEXT NOT NULL,
    record_code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, category, record_code)
);

CREATE INDEX IF NOT EXISTS idx_master_records_org_category
    ON master_records (organization_id, category, is_active, record_code);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000008_stocktakes.up.sql
-- -----------------------------------------------------------------------------
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

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000009_returns.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE return_takehome_items (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    sales_slip_id TEXT NOT NULL,
    sales_line_id TEXT NOT NULL,
    action_type TEXT NOT NULL CHECK(action_type IN ('return','take_home')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','completed','cancelled')),
    quantity INTEGER NOT NULL CHECK(quantity > 0),
    reason TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    requested_by TEXT NOT NULL,
    requested_at TEXT NOT NULL,
    processed_by TEXT,
    processed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (sales_slip_id) REFERENCES sales_slips(id),
    FOREIGN KEY (sales_line_id) REFERENCES sales_lines(id),
    FOREIGN KEY (requested_by) REFERENCES users(id),
    FOREIGN KEY (processed_by) REFERENCES users(id)
);

CREATE INDEX idx_return_takehome_org_status
ON return_takehome_items(organization_id,status,requested_at DESC);

CREATE INDEX idx_return_takehome_sale
ON return_takehome_items(organization_id,sales_slip_id);

CREATE UNIQUE INDEX idx_return_takehome_one_pending_action
ON return_takehome_items(organization_id,sales_line_id,action_type)
WHERE status='pending';

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000010_purchase_sale_price.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE purchase_slip_lines
ADD COLUMN base_sale_price_minor INTEGER NOT NULL DEFAULT 0
CHECK (base_sale_price_minor >= 0);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000011_product_registration_details.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE products ADD COLUMN material_text TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN box_text TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN movement_text TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN belt_material_text TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN dial_text TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN features_text TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN internal_comment_text TEXT NOT NULL DEFAULT '';

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000012_purchase_returns.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE purchase_return_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    purchase_slip_id TEXT,
    return_number TEXT NOT NULL,
    return_date TEXT NOT NULL,
    supplier_name TEXT NOT NULL,
    item_count INTEGER NOT NULL CHECK(item_count > 0),
    amount_jpy INTEGER NOT NULL CHECK(amount_jpy >= 0),
    reason TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','returned','completed')),
    delivery_number TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (purchase_slip_id) REFERENCES purchase_slips(id),
    UNIQUE (organization_id, return_number)
);

CREATE INDEX idx_purchase_returns_org_date
ON purchase_return_slips(organization_id,return_date DESC);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000013_purchase_slip_workflow.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE purchase_slip_revisions (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    purchase_slip_id TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    memo TEXT NOT NULL,
    before_json TEXT NOT NULL,
    after_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (purchase_slip_id) REFERENCES purchase_slips(id),
    FOREIGN KEY (actor_user_id) REFERENCES users(id)
);

CREATE INDEX idx_purchase_slip_revisions_slip
ON purchase_slip_revisions(organization_id,purchase_slip_id,created_at DESC);

ALTER TABLE purchase_return_slips ADD COLUMN notes TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_return_slips ADD COLUMN created_by TEXT REFERENCES users(id);

CREATE TABLE purchase_return_lines (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    purchase_return_slip_id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    product_code TEXT NOT NULL,
    sku TEXT NOT NULL DEFAULT '',
    brand TEXT NOT NULL DEFAULT '',
    model_number TEXT NOT NULL DEFAULT '',
    amount_jpy INTEGER NOT NULL CHECK(amount_jpy >= 0),
    created_at TEXT NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (purchase_return_slip_id) REFERENCES purchase_return_slips(id),
    FOREIGN KEY (product_id) REFERENCES products(id),
    UNIQUE (organization_id,product_id)
);

CREATE INDEX idx_purchase_return_lines_slip
ON purchase_return_lines(organization_id,purchase_return_slip_id);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000014_shipment_slip_workflow.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE shipment_slips ADD COLUMN recipient_address TEXT NOT NULL DEFAULT '';
ALTER TABLE shipment_slips ADD COLUMN recipient_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE shipment_slips ADD COLUMN tracking_number TEXT NOT NULL DEFAULT '';

ALTER TABLE shipment_lines ADD COLUMN wholesale_price_minor INTEGER NOT NULL DEFAULT 0
    CHECK(wholesale_price_minor >= 0);

CREATE TABLE shipment_slip_revisions (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    shipment_slip_id TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    memo TEXT NOT NULL,
    before_json TEXT NOT NULL,
    after_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (shipment_slip_id) REFERENCES shipment_slips(id),
    FOREIGN KEY (actor_user_id) REFERENCES users(id)
);

CREATE INDEX idx_shipment_slip_revisions_slip
ON shipment_slip_revisions(organization_id,shipment_slip_id,created_at DESC);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000015_sales_slip_workflow.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE sales_slips ADD COLUMN customer_address TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_slips ADD COLUMN customer_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_slips ADD COLUMN qualified_invoice_number TEXT NOT NULL DEFAULT '';

ALTER TABLE return_takehome_items ADD COLUMN return_date TEXT NOT NULL DEFAULT '';

CREATE TABLE sales_slip_revisions (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    sales_slip_id TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    memo TEXT NOT NULL,
    before_json TEXT NOT NULL,
    after_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (sales_slip_id) REFERENCES sales_slips(id),
    FOREIGN KEY (actor_user_id) REFERENCES users(id)
);

CREATE INDEX idx_sales_slip_revisions_slip
ON sales_slip_revisions(organization_id,sales_slip_id,created_at DESC);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000016_sales_return_invoice.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE return_takehome_items ADD COLUMN invoice_issued_at TEXT;
ALTER TABLE return_takehome_items ADD COLUMN invoice_issued_by TEXT REFERENCES users(id);
ALTER TABLE return_takehome_items ADD COLUMN invoice_printed_at TEXT;
ALTER TABLE return_takehome_items ADD COLUMN invoice_printed_by TEXT REFERENCES users(id);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000017_purchase_return_invoice.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE purchase_return_slips ADD COLUMN invoice_issued_at TEXT;
ALTER TABLE purchase_return_slips ADD COLUMN invoice_issued_by TEXT REFERENCES users(id);
ALTER TABLE purchase_return_slips ADD COLUMN invoice_printed_at TEXT;
ALTER TABLE purchase_return_slips ADD COLUMN invoice_printed_by TEXT REFERENCES users(id);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000018_purchase_line_product_details.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE purchase_slip_lines ADD COLUMN requested_product_code TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN sku TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN serial_number TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN material_text TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN movement_text TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN condition_text TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN belt_material_text TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN dial_text TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN box_text TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN accessories TEXT NOT NULL DEFAULT '';
ALTER TABLE purchase_slip_lines ADD COLUMN features_text TEXT NOT NULL DEFAULT '';

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000019_return_inventory_restore.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE return_takehome_items ADD COLUMN inventory_restored_at TEXT;
ALTER TABLE return_takehome_items ADD COLUMN inventory_restored_by TEXT REFERENCES users(id);
ALTER TABLE return_takehome_items ADD COLUMN restore_box_text TEXT NOT NULL DEFAULT '';
ALTER TABLE return_takehome_items ADD COLUMN restore_comment_text TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_return_takehome_restore
ON return_takehome_items(organization_id,inventory_restored_at,sales_slip_id);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000020_phase8_master_alignment.up.sql
-- -----------------------------------------------------------------------------
-- sales_lines.exchange_rate_snapshot_id already references this table on
-- upgraded installations.  Defer that relationship until the replacement
-- table has been renamed back to exchange_rate_snapshots at commit time.
PRAGMA defer_foreign_keys = ON;

ALTER TABLE suppliers ADD COLUMN address TEXT NOT NULL DEFAULT '';
ALTER TABLE suppliers ADD COLUMN contact TEXT NOT NULL DEFAULT '';
ALTER TABLE suppliers ADD COLUMN invoice_registration_number TEXT NOT NULL DEFAULT '';

ALTER TABLE master_records ADD COLUMN address TEXT NOT NULL DEFAULT '';
ALTER TABLE master_records ADD COLUMN contact TEXT NOT NULL DEFAULT '';
ALTER TABLE master_records ADD COLUMN invoice_registration_number TEXT NOT NULL DEFAULT '';
ALTER TABLE master_records ADD COLUMN details_json TEXT NOT NULL DEFAULT '{}';

DROP INDEX IF EXISTS idx_exchange_rates_latest;
CREATE TABLE exchange_rate_snapshots_phase8 (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    base_currency TEXT NOT NULL CHECK (base_currency IN ('USD', 'EUR', 'HKD', 'CHF')),
    quote_currency TEXT NOT NULL CHECK (quote_currency = 'JPY'),
    rate_scaled INTEGER NOT NULL CHECK (rate_scaled > 0),
    scale INTEGER NOT NULL DEFAULT 100000000,
    provider TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    CHECK (base_currency <> quote_currency)
);
INSERT INTO exchange_rate_snapshots_phase8
SELECT id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,observed_at,created_by,created_at
FROM exchange_rate_snapshots
WHERE base_currency='USD' AND quote_currency='JPY';
-- JPY/USD snapshots cannot satisfy the Phase 8 USD-per-JPY master contract.
-- Sales lines retain their captured numeric rate, scale and observation time,
-- so only detach the obsolete parent identifier before replacing the master.
UPDATE sales_lines
SET exchange_rate_snapshot_id = NULL
WHERE exchange_rate_snapshot_id IN (
    SELECT id
    FROM exchange_rate_snapshots
    WHERE NOT (base_currency='USD' AND quote_currency='JPY')
);
DROP TABLE exchange_rate_snapshots;
ALTER TABLE exchange_rate_snapshots_phase8 RENAME TO exchange_rate_snapshots;
CREATE INDEX idx_exchange_rates_latest
    ON exchange_rate_snapshots (organization_id, base_currency, quote_currency, observed_at DESC);

CREATE TABLE IF NOT EXISTS guest_companies (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, company_code)
);

CREATE INDEX IF NOT EXISTS idx_guest_companies_org_active
    ON guest_companies (organization_id, is_active, company_code);

CREATE TABLE IF NOT EXISTS guest_boxes (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    box_number INTEGER NOT NULL CHECK (box_number BETWEEN 1 AND 10),
    box_name TEXT NOT NULL DEFAULT '',
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, box_number)
);

CREATE TABLE IF NOT EXISTS guest_box_drafts (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    is_selected INTEGER NOT NULL DEFAULT 0 CHECK (is_selected IN (0, 1)),
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, company_id, box_id)
);

CREATE TABLE IF NOT EXISTS guest_box_publications (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    is_published INTEGER NOT NULL DEFAULT 0 CHECK (is_published IN (0, 1)),
    published_by TEXT REFERENCES users(id),
    published_at TEXT,
    PRIMARY KEY (organization_id, company_id, box_id)
);

CREATE TABLE IF NOT EXISTS guest_box_products (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    sort_order INTEGER NOT NULL DEFAULT 0,
    added_by TEXT REFERENCES users(id),
    added_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, box_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_guest_box_products_box
    ON guest_box_products (organization_id, box_id, sort_order, product_id);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000021_product_market_prices.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS product_market_prices (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    purchase_market_price_minor INTEGER NOT NULL DEFAULT 0 CHECK (purchase_market_price_minor >= 0),
    sale_market_price_minor INTEGER NOT NULL DEFAULT 0 CHECK (sale_market_price_minor >= 0),
    updated_by TEXT NOT NULL REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_product_market_prices_product
    ON product_market_prices (organization_id, product_id);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000022_phase8_publish_snapshot.up.sql
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS guest_box_published_products (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    sort_order INTEGER NOT NULL DEFAULT 0,
    published_by TEXT REFERENCES users(id),
    published_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, company_id, box_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_guest_box_published_products_lookup
    ON guest_box_published_products (organization_id, company_id, box_id, sort_order, product_id);

CREATE TABLE IF NOT EXISTS guest_credentials (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    guest_id TEXT NOT NULL,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, company_id),
    UNIQUE (organization_id, guest_id)
);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000024_guest_snapshot_details.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE guest_box_published_products ADD COLUMN product_code TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN brand TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN model_name TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN reference_number TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN serial_number TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN sale_price_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE guest_box_published_products ADD COLUMN sale_currency TEXT NOT NULL DEFAULT 'JPY';
ALTER TABLE guest_box_published_products ADD COLUMN condition_text TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN accessories TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN inventory_status TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN publication_status TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN box_name TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS guest_box_published_images (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    image_id TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    original_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    published_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, company_id, box_id, product_id, image_id)
);

CREATE INDEX IF NOT EXISTS idx_guest_box_published_images_lookup
    ON guest_box_published_images (organization_id, company_id, product_id, sort_order, image_id);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000025_stocktake_mock_alignment.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE stocktakes ADD COLUMN expected_total_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE stocktakes ADD COLUMN saved_at TEXT;

ALTER TABLE stocktake_lines ADD COLUMN difference_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE stocktake_lines ADD COLUMN review_status TEXT NOT NULL DEFAULT 'none'
    CHECK (review_status IN ('none', 'pending', 'approved'));
ALTER TABLE stocktake_lines ADD COLUMN finalized_at TEXT;

UPDATE stocktakes
SET expected_total_minor = COALESCE((
    SELECT SUM(p.cost_amount_minor)
    FROM stocktake_lines sl
    JOIN products p ON p.id = sl.product_id
    WHERE sl.stocktake_id = stocktakes.id
), 0);

CREATE INDEX IF NOT EXISTS idx_stocktake_lines_review
    ON stocktake_lines (stocktake_id, review_status, counted_present);

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000026_stocktake_draft_uniqueness.up.sql
-- -----------------------------------------------------------------------------
CREATE UNIQUE INDEX IF NOT EXISTS ux_stocktakes_one_draft_per_org
    ON stocktakes (organization_id)
    WHERE status = 'draft';

-- -----------------------------------------------------------------------------
-- LEGACY SOURCE: 000027_purchase_request_groups.up.sql
-- -----------------------------------------------------------------------------
ALTER TABLE purchase_requests
    ADD COLUMN request_group_id TEXT NOT NULL DEFAULT '';

UPDATE purchase_requests
SET request_group_id = id
WHERE request_group_id = '';

CREATE INDEX IF NOT EXISTS idx_purchase_requests_org_group_status
    ON purchase_requests (organization_id, request_group_id, status, requested_at DESC);

-- End of candidate. Run PRAGMA foreign_key_check separately in local SQLite.
