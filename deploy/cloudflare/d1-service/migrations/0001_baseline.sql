-- GENERATED D1 TEST BASELINE THROUGH LEGACY MIGRATION 000027.
-- Empty database bootstrap only. No seed data. Never apply to the current SQLite database.

-- table: approval_actions
CREATE TABLE approval_actions (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    approval_request_id TEXT NOT NULL REFERENCES approval_requests(id),
    actor_user_id TEXT NOT NULL REFERENCES users(id),
    action TEXT NOT NULL CHECK (action IN ('requested', 'approved', 'returned', 'rejected', 'cancelled', 'expired')),
    comment TEXT NOT NULL DEFAULT '',
    acted_at TEXT NOT NULL
);

-- table: approval_requests
CREATE TABLE approval_requests (
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

-- table: audit_logs
CREATE TABLE audit_logs (
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

-- table: exchange_rate_snapshots
CREATE TABLE "exchange_rate_snapshots" (
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

-- table: guest_box_drafts
CREATE TABLE guest_box_drafts (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    is_selected INTEGER NOT NULL DEFAULT 0 CHECK (is_selected IN (0, 1)),
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, company_id, box_id)
);

-- table: guest_box_products
CREATE TABLE guest_box_products (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    sort_order INTEGER NOT NULL DEFAULT 0,
    added_by TEXT REFERENCES users(id),
    added_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, box_id, product_id)
);

-- table: guest_box_publications
CREATE TABLE guest_box_publications (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    is_published INTEGER NOT NULL DEFAULT 0 CHECK (is_published IN (0, 1)),
    published_by TEXT REFERENCES users(id),
    published_at TEXT,
    PRIMARY KEY (organization_id, company_id, box_id)
);

-- table: guest_box_published_images
CREATE TABLE guest_box_published_images (
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

-- table: guest_box_published_products
CREATE TABLE guest_box_published_products (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    sort_order INTEGER NOT NULL DEFAULT 0,
    published_by TEXT REFERENCES users(id),
    published_at TEXT NOT NULL, product_code TEXT NOT NULL DEFAULT '', brand TEXT NOT NULL DEFAULT '', model_name TEXT NOT NULL DEFAULT '', reference_number TEXT NOT NULL DEFAULT '', serial_number TEXT NOT NULL DEFAULT '', sale_price_minor INTEGER NOT NULL DEFAULT 0, sale_currency TEXT NOT NULL DEFAULT 'JPY', condition_text TEXT NOT NULL DEFAULT '', accessories TEXT NOT NULL DEFAULT '', inventory_status TEXT NOT NULL DEFAULT '', publication_status TEXT NOT NULL DEFAULT '', box_name TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (organization_id, company_id, box_id, product_id)
);

-- table: guest_boxes
CREATE TABLE guest_boxes (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    box_number INTEGER NOT NULL CHECK (box_number BETWEEN 1 AND 10),
    box_name TEXT NOT NULL DEFAULT '',
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, box_number)
);

-- table: guest_companies
CREATE TABLE guest_companies (
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

-- table: guest_credentials
CREATE TABLE guest_credentials (
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

-- table: inventory_events
CREATE TABLE inventory_events (
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

-- table: login_csrf_tokens
CREATE TABLE login_csrf_tokens (
    token_hash TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- table: market_import_batches
CREATE TABLE market_import_batches (
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

-- table: market_import_rows
CREATE TABLE market_import_rows (
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

-- table: market_price_records
CREATE TABLE market_price_records (
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

-- table: master_records
CREATE TABLE master_records (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    category TEXT NOT NULL,
    record_code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL, address TEXT NOT NULL DEFAULT '', contact TEXT NOT NULL DEFAULT '', invoice_registration_number TEXT NOT NULL DEFAULT '', details_json TEXT NOT NULL DEFAULT '{}',
    UNIQUE (organization_id, category, record_code)
);

-- table: organization_settings
CREATE TABLE organization_settings (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    setting_key TEXT NOT NULL,
    setting_value TEXT NOT NULL,
    value_type TEXT NOT NULL DEFAULT 'string',
    is_configured INTEGER NOT NULL DEFAULT 0,
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, setting_key)
);

-- table: organizations
CREATE TABLE organizations (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- table: permissions
CREATE TABLE permissions (
    permission_key TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

-- table: product_code_sequences
CREATE TABLE product_code_sequences (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    purchase_date TEXT NOT NULL,
    last_sequence INTEGER NOT NULL CHECK (last_sequence BETWEEN 0 AND 999),
    PRIMARY KEY (organization_id, purchase_date)
);

-- table: product_images
CREATE TABLE product_images (
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

-- table: product_market_prices
CREATE TABLE product_market_prices (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    purchase_market_price_minor INTEGER NOT NULL DEFAULT 0 CHECK (purchase_market_price_minor >= 0),
    sale_market_price_minor INTEGER NOT NULL DEFAULT 0 CHECK (sale_market_price_minor >= 0),
    updated_by TEXT NOT NULL REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, product_id)
);

-- table: products
CREATE TABLE products (
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
    updated_at TEXT NOT NULL, material_text TEXT NOT NULL DEFAULT '', box_text TEXT NOT NULL DEFAULT '', movement_text TEXT NOT NULL DEFAULT '', belt_material_text TEXT NOT NULL DEFAULT '', dial_text TEXT NOT NULL DEFAULT '', features_text TEXT NOT NULL DEFAULT '', internal_comment_text TEXT NOT NULL DEFAULT '',
    UNIQUE (organization_id, product_code)
);

-- table: purchase_requests
CREATE TABLE purchase_requests (
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
    updated_at TEXT NOT NULL, request_group_id TEXT NOT NULL DEFAULT '',
    UNIQUE (organization_id, request_number)
);

-- table: purchase_return_lines
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

-- table: purchase_return_slips
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
    updated_at TEXT NOT NULL, notes TEXT NOT NULL DEFAULT '', created_by TEXT REFERENCES users(id), invoice_issued_at TEXT, invoice_issued_by TEXT REFERENCES users(id), invoice_printed_at TEXT, invoice_printed_by TEXT REFERENCES users(id),
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (purchase_slip_id) REFERENCES purchase_slips(id),
    UNIQUE (organization_id, return_number)
);

-- table: purchase_slip_lines
CREATE TABLE purchase_slip_lines (
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
    created_at TEXT NOT NULL, base_sale_price_minor INTEGER NOT NULL DEFAULT 0
CHECK (base_sale_price_minor >= 0), requested_product_code TEXT NOT NULL DEFAULT '', sku TEXT NOT NULL DEFAULT '', serial_number TEXT NOT NULL DEFAULT '', material_text TEXT NOT NULL DEFAULT '', movement_text TEXT NOT NULL DEFAULT '', condition_text TEXT NOT NULL DEFAULT '', belt_material_text TEXT NOT NULL DEFAULT '', dial_text TEXT NOT NULL DEFAULT '', box_text TEXT NOT NULL DEFAULT '', accessories TEXT NOT NULL DEFAULT '', features_text TEXT NOT NULL DEFAULT '',
    UNIQUE (purchase_slip_id, line_number)
);

-- table: purchase_slip_revisions
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

-- table: purchase_slips
CREATE TABLE purchase_slips (
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

-- table: reservations
CREATE TABLE reservations (
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

-- table: return_takehome_items
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
    updated_at TEXT NOT NULL, return_date TEXT NOT NULL DEFAULT '', invoice_issued_at TEXT, invoice_issued_by TEXT REFERENCES users(id), invoice_printed_at TEXT, invoice_printed_by TEXT REFERENCES users(id), inventory_restored_at TEXT, inventory_restored_by TEXT REFERENCES users(id), restore_box_text TEXT NOT NULL DEFAULT '', restore_comment_text TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (sales_slip_id) REFERENCES sales_slips(id),
    FOREIGN KEY (sales_line_id) REFERENCES sales_lines(id),
    FOREIGN KEY (requested_by) REFERENCES users(id),
    FOREIGN KEY (processed_by) REFERENCES users(id)
);

-- table: role_permissions
CREATE TABLE role_permissions (
    role_key TEXT NOT NULL REFERENCES roles(role_key),
    permission_key TEXT NOT NULL REFERENCES permissions(permission_key),
    PRIMARY KEY (role_key, permission_key)
);

-- table: roles
CREATE TABLE roles (
    role_key TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL
);

-- table: sales_lines
CREATE TABLE sales_lines (
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

-- table: sales_shipment_allocations
CREATE TABLE sales_shipment_allocations (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    sales_line_id TEXT NOT NULL REFERENCES sales_lines(id),
    shipment_line_id TEXT NOT NULL REFERENCES shipment_lines(id),
    allocated_quantity INTEGER NOT NULL CHECK (allocated_quantity > 0),
    created_at TEXT NOT NULL,
    UNIQUE (sales_line_id, shipment_line_id)
);

-- table: sales_slip_revisions
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

-- table: sales_slips
CREATE TABLE sales_slips (
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
    updated_at TEXT NOT NULL, customer_address TEXT NOT NULL DEFAULT '', customer_phone TEXT NOT NULL DEFAULT '', qualified_invoice_number TEXT NOT NULL DEFAULT '',
    UNIQUE (organization_id, slip_number)
);

-- table: serial_duplicate_overrides
CREATE TABLE serial_duplicate_overrides (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    serial_number TEXT NOT NULL,
    candidate_product_ids_json TEXT NOT NULL,
    reason TEXT NOT NULL,
    actor_user_id TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL
);

-- table: sessions
CREATE TABLE sessions (
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

-- table: shipment_lines
CREATE TABLE shipment_lines (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    shipment_slip_id TEXT NOT NULL REFERENCES shipment_slips(id),
    line_number INTEGER NOT NULL,
    product_id TEXT NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    created_at TEXT NOT NULL, wholesale_price_minor INTEGER NOT NULL DEFAULT 0
    CHECK(wholesale_price_minor >= 0),
    UNIQUE (shipment_slip_id, line_number),
    UNIQUE (shipment_slip_id, product_id)
);

-- table: shipment_slip_revisions
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

-- table: shipment_slips
CREATE TABLE shipment_slips (
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
    updated_at TEXT NOT NULL, recipient_address TEXT NOT NULL DEFAULT '', recipient_phone TEXT NOT NULL DEFAULT '', tracking_number TEXT NOT NULL DEFAULT '',
    UNIQUE (organization_id, shipment_number)
);

-- table: stocktake_lines
CREATE TABLE stocktake_lines (
    id TEXT PRIMARY KEY,
    stocktake_id TEXT NOT NULL REFERENCES stocktakes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    expected_present INTEGER NOT NULL DEFAULT 1 CHECK (expected_present IN (0, 1)),
    counted_present INTEGER CHECK (counted_present IN (0, 1)),
    notes TEXT NOT NULL DEFAULT '',
    counted_by TEXT REFERENCES users(id),
    counted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL, difference_reason TEXT NOT NULL DEFAULT '', review_status TEXT NOT NULL DEFAULT 'none'
    CHECK (review_status IN ('none', 'pending', 'approved')), finalized_at TEXT,
    UNIQUE (stocktake_id, product_id)
);

-- table: stocktakes
CREATE TABLE stocktakes (
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
    completed_at TEXT, expected_total_minor INTEGER NOT NULL DEFAULT 0, saved_at TEXT,
    UNIQUE (organization_id, stocktake_number)
);

-- table: suppliers
CREATE TABLE suppliers (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    supplier_code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL, address TEXT NOT NULL DEFAULT '', contact TEXT NOT NULL DEFAULT '', invoice_registration_number TEXT NOT NULL DEFAULT '',
    UNIQUE (organization_id, supplier_code)
);

-- table: user_permissions
CREATE TABLE user_permissions (
    user_id TEXT NOT NULL REFERENCES users(id),
    permission_key TEXT NOT NULL REFERENCES permissions(permission_key),
    effect TEXT NOT NULL CHECK (effect IN ('allow', 'deny')),
    PRIMARY KEY (user_id, permission_key)
);

-- table: users
CREATE TABLE users (
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

-- index: idx_allocations_sales_line
CREATE INDEX idx_allocations_sales_line
    ON sales_shipment_allocations (organization_id, sales_line_id);

-- index: idx_approval_actions_request
CREATE INDEX idx_approval_actions_request
    ON approval_actions (organization_id, approval_request_id, acted_at);

-- index: idx_approvals_org_status
CREATE INDEX idx_approvals_org_status
    ON approval_requests (organization_id, status, requested_at DESC);

-- index: idx_audit_org_created
CREATE INDEX idx_audit_org_created
    ON audit_logs (organization_id, created_at DESC);

-- index: idx_exchange_rates_latest
CREATE INDEX idx_exchange_rates_latest
    ON exchange_rate_snapshots (organization_id, base_currency, quote_currency, observed_at DESC);

-- index: idx_guest_box_products_box
CREATE INDEX idx_guest_box_products_box
    ON guest_box_products (organization_id, box_id, sort_order, product_id);

-- index: idx_guest_box_published_images_lookup
CREATE INDEX idx_guest_box_published_images_lookup
    ON guest_box_published_images (organization_id, company_id, product_id, sort_order, image_id);

-- index: idx_guest_box_published_products_lookup
CREATE INDEX idx_guest_box_published_products_lookup
    ON guest_box_published_products (organization_id, company_id, box_id, sort_order, product_id);

-- index: idx_guest_companies_org_active
CREATE INDEX idx_guest_companies_org_active
    ON guest_companies (organization_id, is_active, company_code);

-- index: idx_inventory_events_product
CREATE INDEX idx_inventory_events_product
    ON inventory_events (organization_id, product_id, created_at DESC);

-- index: idx_login_csrf_expiry
CREATE INDEX idx_login_csrf_expiry
    ON login_csrf_tokens (expires_at);

-- index: idx_market_import_rows_batch
CREATE INDEX idx_market_import_rows_batch
    ON market_import_rows (batch_id, row_number);

-- index: idx_market_prices_lookup
CREATE INDEX idx_market_prices_lookup
    ON market_price_records (organization_id, brand, model_number, currency, market_date DESC);

-- index: idx_market_prices_org_date
CREATE INDEX idx_market_prices_org_date
    ON market_price_records (organization_id, market_date DESC);

-- index: idx_master_records_org_category
CREATE INDEX idx_master_records_org_category
    ON master_records (organization_id, category, is_active, record_code);

-- index: idx_product_images_product
CREATE INDEX idx_product_images_product
    ON product_images (organization_id, product_id, sort_order);

-- index: idx_product_market_prices_product
CREATE INDEX idx_product_market_prices_product
    ON product_market_prices (organization_id, product_id);

-- index: idx_products_org_serial
CREATE INDEX idx_products_org_serial
    ON products (organization_id, serial_number);

-- index: idx_products_org_sku
CREATE INDEX idx_products_org_sku
    ON products (organization_id, sku);

-- index: idx_products_org_status
CREATE INDEX idx_products_org_status
    ON products (organization_id, inventory_status, purchase_date DESC);

-- index: idx_purchase_requests_org_group_status
CREATE INDEX idx_purchase_requests_org_group_status
    ON purchase_requests (organization_id, request_group_id, status, requested_at DESC);

-- index: idx_purchase_requests_org_status
CREATE INDEX idx_purchase_requests_org_status
    ON purchase_requests (organization_id, status, requested_at DESC);

-- index: idx_purchase_requests_product
CREATE INDEX idx_purchase_requests_product
    ON purchase_requests (organization_id, product_id, status);

-- index: idx_purchase_return_lines_slip
CREATE INDEX idx_purchase_return_lines_slip
ON purchase_return_lines(organization_id,purchase_return_slip_id);

-- index: idx_purchase_returns_org_date
CREATE INDEX idx_purchase_returns_org_date
ON purchase_return_slips(organization_id,return_date DESC);

-- index: idx_purchase_slip_revisions_slip
CREATE INDEX idx_purchase_slip_revisions_slip
ON purchase_slip_revisions(organization_id,purchase_slip_id,created_at DESC);

-- index: idx_purchase_slips_org_date
CREATE INDEX idx_purchase_slips_org_date
    ON purchase_slips (organization_id, purchase_date DESC);

-- index: idx_reservations_expiry
CREATE INDEX idx_reservations_expiry
    ON reservations (organization_id, status, expires_at);

-- index: idx_return_takehome_one_pending_action
CREATE UNIQUE INDEX idx_return_takehome_one_pending_action
ON return_takehome_items(organization_id,sales_line_id,action_type)
WHERE status='pending';

-- index: idx_return_takehome_org_status
CREATE INDEX idx_return_takehome_org_status
ON return_takehome_items(organization_id,status,requested_at DESC);

-- index: idx_return_takehome_restore
CREATE INDEX idx_return_takehome_restore
ON return_takehome_items(organization_id,inventory_restored_at,sales_slip_id);

-- index: idx_return_takehome_sale
CREATE INDEX idx_return_takehome_sale
ON return_takehome_items(organization_id,sales_slip_id);

-- index: idx_sales_lines_product
CREATE INDEX idx_sales_lines_product
    ON sales_lines (organization_id, product_id);

-- index: idx_sales_slip_revisions_slip
CREATE INDEX idx_sales_slip_revisions_slip
ON sales_slip_revisions(organization_id,sales_slip_id,created_at DESC);

-- index: idx_sales_slips_org_date
CREATE INDEX idx_sales_slips_org_date
    ON sales_slips (organization_id, sales_date DESC);

-- index: idx_sessions_expiry
CREATE INDEX idx_sessions_expiry ON sessions (expires_at);

-- index: idx_sessions_user
CREATE INDEX idx_sessions_user ON sessions (user_id);

-- index: idx_shipment_lines_product
CREATE INDEX idx_shipment_lines_product
    ON shipment_lines (organization_id, product_id);

-- index: idx_shipment_slip_revisions_slip
CREATE INDEX idx_shipment_slip_revisions_slip
ON shipment_slip_revisions(organization_id,shipment_slip_id,created_at DESC);

-- index: idx_shipments_org_date
CREATE INDEX idx_shipments_org_date
    ON shipment_slips (organization_id, shipment_date DESC);

-- index: idx_stocktake_lines_review
CREATE INDEX idx_stocktake_lines_review
    ON stocktake_lines (stocktake_id, review_status, counted_present);

-- index: idx_stocktake_lines_stocktake
CREATE INDEX idx_stocktake_lines_stocktake
    ON stocktake_lines (stocktake_id, counted_present, product_id);

-- index: idx_stocktakes_org_date
CREATE INDEX idx_stocktakes_org_date
    ON stocktakes (organization_id, stocktake_date DESC, created_at DESC);

-- index: idx_users_org_active
CREATE INDEX idx_users_org_active
    ON users (organization_id, is_active);

-- index: uq_active_reservation_product
CREATE UNIQUE INDEX uq_active_reservation_product
    ON reservations (organization_id, product_id)
    WHERE status='active';

-- index: uq_pending_approval_target
CREATE UNIQUE INDEX uq_pending_approval_target
    ON approval_requests (organization_id, target_type, target_id, action_key)
    WHERE status='pending';

-- index: ux_stocktakes_one_draft_per_org
CREATE UNIQUE INDEX ux_stocktakes_one_draft_per_org
    ON stocktakes (organization_id)
    WHERE status = 'draft';

-- trigger: audit_logs_no_delete
CREATE TRIGGER audit_logs_no_delete
BEFORE DELETE ON audit_logs
BEGIN
    SELECT RAISE(ABORT, 'audit logs are immutable');
END;
-- trigger: audit_logs_no_update
CREATE TRIGGER audit_logs_no_update
BEFORE UPDATE ON audit_logs
BEGIN
    SELECT RAISE(ABORT, 'audit logs are immutable');
END;
