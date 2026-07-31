-- PostgreSQL 16+ initial physical schema candidate for the AWS production path.
-- CANDIDATE ONLY: never apply directly to production. Validate on an isolated
-- staging RDS database with the runbook in docs/db-api/11-postgres-production-design.md.
--
-- This is a fresh-database baseline, not an in-place conversion of the SQLite
-- migration chain. Data import is a separate, one-way, reconciled operation.

\set ON_ERROR_STOP on

\if :{?migration_checksum}
\else
    \echo 'migration_checksum is required'
    \quit
\endif

\if :{?execution_id}
\else
    \echo 'execution_id is required'
    \quit
\endif

BEGIN;

SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '10min';
SET LOCAL idle_in_transaction_session_timeout = '2min';
SET LOCAL TIME ZONE 'UTC';

SELECT pg_advisory_xact_lock(hashtextextended('zaiko:postgres:migration', 0));

CREATE SCHEMA zaiko;

CREATE TABLE zaiko.schema_migrations (
    version                 BIGINT PRIMARY KEY,
    name                    TEXT NOT NULL UNIQUE,
    checksum_sha256         CHAR(64) NOT NULL CHECK (checksum_sha256 ~ '^[0-9a-f]{64}$'),
    applied_at              TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    applied_by              TEXT NOT NULL DEFAULT CURRENT_USER,
    execution_id            TEXT NOT NULL,
    execution_duration_ms   BIGINT NOT NULL CHECK (execution_duration_ms >= 0)
);

CREATE TABLE zaiko.organizations (
    id          TEXT PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    version     BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (btrim(id) <> ''),
    CHECK (btrim(code) <> ''),
    CHECK (btrim(name) <> '')
);

CREATE TABLE zaiko.permissions (
    permission_key  TEXT PRIMARY KEY,
    description     TEXT NOT NULL
);

CREATE TABLE zaiko.roles (
    role_key     TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL
);

CREATE TABLE zaiko.role_permissions (
    role_key        TEXT NOT NULL REFERENCES zaiko.roles(role_key) ON DELETE CASCADE,
    permission_key  TEXT NOT NULL REFERENCES zaiko.permissions(permission_key) ON DELETE CASCADE,
    PRIMARY KEY (role_key, permission_key)
);

CREATE TABLE zaiko.users (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    username         TEXT NOT NULL,
    password_hash    TEXT NOT NULL,
    display_name     TEXT NOT NULL,
    role_key         TEXT NOT NULL REFERENCES zaiko.roles(role_key),
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    version          BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    last_login_at    TIMESTAMPTZ,
    deleted_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, username),
    CHECK (btrim(username) <> ''),
    CHECK (btrim(password_hash) <> ''),
    CHECK (btrim(display_name) <> '')
);

CREATE INDEX idx_users_org_active
    ON zaiko.users (organization_id, is_active, username)
    WHERE deleted_at IS NULL;

CREATE TABLE zaiko.user_permissions (
    organization_id  TEXT NOT NULL,
    user_id           TEXT NOT NULL,
    permission_key    TEXT NOT NULL REFERENCES zaiko.permissions(permission_key),
    effect            TEXT NOT NULL CHECK (effect IN ('allow', 'deny')),
    PRIMARY KEY (organization_id, user_id, permission_key),
    FOREIGN KEY (organization_id, user_id)
        REFERENCES zaiko.users(organization_id, id) ON DELETE CASCADE
);

INSERT INTO zaiko.permissions (permission_key, description) VALUES
    ('inventory.write', '在庫更新'),
    ('purchase.confirm', '仕入確定'),
    ('sales.confirm', '売上確定'),
    ('shipment.confirm', '出荷確定');

CREATE TABLE zaiko.organization_settings (
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    setting_key      TEXT NOT NULL,
    setting_value    JSONB NOT NULL DEFAULT 'null'::jsonb,
    value_type       TEXT NOT NULL DEFAULT 'string',
    is_configured    BOOLEAN NOT NULL DEFAULT FALSE,
    version          BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by       TEXT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, setting_key),
    FOREIGN KEY (organization_id, updated_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE TABLE zaiko.sessions (
    token_hash       CHAR(64) PRIMARY KEY CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    organization_id TEXT NOT NULL,
    user_id          TEXT NOT NULL,
    csrf_token_hash  CHAR(64) NOT NULL CHECK (csrf_token_hash ~ '^[0-9a-f]{64}$'),
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ip_address       INET,
    user_agent       TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (organization_id, user_id)
        REFERENCES zaiko.users(organization_id, id) ON DELETE CASCADE,
    CHECK (expires_at > created_at)
);

CREATE INDEX idx_sessions_expiry ON zaiko.sessions (expires_at);
CREATE INDEX idx_sessions_user ON zaiko.sessions (organization_id, user_id);

CREATE TABLE zaiko.login_csrf_tokens (
    token_hash   CHAR(64) PRIMARY KEY CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (expires_at > created_at)
);

CREATE INDEX idx_login_csrf_expiry ON zaiko.login_csrf_tokens (expires_at);

CREATE TABLE zaiko.idempotency_records (
    organization_id       TEXT NOT NULL REFERENCES zaiko.organizations(id),
    operation_name        TEXT NOT NULL,
    idempotency_key       TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    canonical_request_hash CHAR(64) NOT NULL CHECK (canonical_request_hash ~ '^[0-9a-f]{64}$'),
    state                 TEXT NOT NULL DEFAULT 'committed'
                          CHECK (state IN ('processing', 'committed', 'failed')),
    result_id             TEXT NOT NULL DEFAULT '',
    result_number         TEXT NOT NULL DEFAULT '',
    result_version        BIGINT NOT NULL DEFAULT 0 CHECK (result_version >= 0),
    response_json         JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_user_id         TEXT NOT NULL,
    requested_at          TIMESTAMPTZ NOT NULL,
    committed_at          TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, operation_name, idempotency_key),
    FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES zaiko.users(organization_id, id),
    CHECK (
        (state = 'committed' AND committed_at IS NOT NULL)
        OR state <> 'committed'
    )
);

CREATE INDEX idx_idempotency_expiry
    ON zaiko.idempotency_records (expires_at)
    WHERE expires_at IS NOT NULL;

CREATE TABLE zaiko.business_number_sequences (
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    sequence_kind    TEXT NOT NULL,
    date_key         DATE NOT NULL,
    last_sequence    BIGINT NOT NULL CHECK (last_sequence >= 0),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, sequence_kind, date_key)
);

CREATE TABLE zaiko.audit_logs (
    id                 TEXT PRIMARY KEY,
    organization_id    TEXT REFERENCES zaiko.organizations(id),
    actor_user_id       TEXT,
    applicant_user_id   TEXT,
    approver_user_id    TEXT,
    target_type         TEXT NOT NULL,
    target_id           TEXT NOT NULL,
    action              TEXT NOT NULL,
    before_json         JSONB NOT NULL DEFAULT '{}'::jsonb,
    after_json          JSONB NOT NULL DEFAULT '{}'::jsonb,
    reason              TEXT NOT NULL DEFAULT '',
    comment             TEXT NOT NULL DEFAULT '',
    ip_address          INET,
    user_agent          TEXT NOT NULL DEFAULT '',
    request_id          TEXT NOT NULL,
    idempotency_key     TEXT NOT NULL DEFAULT '',
    result              TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, applicant_user_id)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, approver_user_id)
        REFERENCES zaiko.users(organization_id, id),
    CHECK (btrim(target_type) <> ''),
    CHECK (btrim(target_id) <> ''),
    CHECK (btrim(action) <> ''),
    CHECK (btrim(request_id) <> '')
);

CREATE INDEX idx_audit_org_created
    ON zaiko.audit_logs (organization_id, created_at DESC, id DESC);
CREATE INDEX idx_audit_target
    ON zaiko.audit_logs (organization_id, target_type, target_id, created_at DESC);

CREATE FUNCTION zaiko.prevent_audit_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only';
END;
$$;

CREATE TRIGGER audit_logs_no_update
BEFORE UPDATE ON zaiko.audit_logs
FOR EACH ROW EXECUTE FUNCTION zaiko.prevent_audit_mutation();

CREATE TRIGGER audit_logs_no_delete
BEFORE DELETE ON zaiko.audit_logs
FOR EACH ROW EXECUTE FUNCTION zaiko.prevent_audit_mutation();

CREATE TABLE zaiko.suppliers (
    id                           TEXT PRIMARY KEY,
    organization_id              TEXT NOT NULL REFERENCES zaiko.organizations(id),
    supplier_code                TEXT NOT NULL,
    name                         TEXT NOT NULL,
    address                      TEXT NOT NULL DEFAULT '',
    contact                      TEXT NOT NULL DEFAULT '',
    invoice_registration_number  TEXT NOT NULL DEFAULT '',
    is_active                    BOOLEAN NOT NULL DEFAULT TRUE,
    version                      BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, supplier_code),
    CHECK (btrim(supplier_code) <> ''),
    CHECK (btrim(name) <> '')
);

CREATE TABLE zaiko.master_records (
    id                           TEXT PRIMARY KEY,
    organization_id              TEXT NOT NULL REFERENCES zaiko.organizations(id),
    category                     TEXT NOT NULL,
    record_code                  TEXT NOT NULL,
    name                         TEXT NOT NULL,
    address                      TEXT NOT NULL DEFAULT '',
    contact                      TEXT NOT NULL DEFAULT '',
    invoice_registration_number  TEXT NOT NULL DEFAULT '',
    details_json                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active                    BOOLEAN NOT NULL DEFAULT TRUE,
    version                      BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by                   TEXT,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by                   TEXT,
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, category, record_code),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, updated_by)
        REFERENCES zaiko.users(organization_id, id),
    CHECK (btrim(category) <> ''),
    CHECK (btrim(record_code) <> ''),
    CHECK (btrim(name) <> '')
);

CREATE INDEX idx_master_records_org_category
    ON zaiko.master_records (organization_id, category, is_active, record_code);

CREATE TABLE zaiko.exchange_rate_snapshots (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    base_currency    CHAR(3) NOT NULL CHECK (base_currency ~ '^[A-Z]{3}$'),
    quote_currency   CHAR(3) NOT NULL CHECK (quote_currency ~ '^[A-Z]{3}$'),
    rate_scaled      BIGINT NOT NULL CHECK (rate_scaled > 0),
    scale            BIGINT NOT NULL DEFAULT 100000000 CHECK (scale > 0),
    provider         TEXT NOT NULL,
    observed_at      TIMESTAMPTZ NOT NULL,
    created_by       TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id),
    CHECK (base_currency <> quote_currency)
);

CREATE INDEX idx_exchange_rates_latest
    ON zaiko.exchange_rate_snapshots
       (organization_id, base_currency, quote_currency, observed_at DESC);

CREATE TABLE zaiko.purchase_slips (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    slip_number      TEXT NOT NULL,
    supplier_id      TEXT NOT NULL,
    purchase_date    DATE NOT NULL,
    status           TEXT NOT NULL CHECK (status IN ('draft', 'confirmed', 'cancelled')),
    is_simple        BOOLEAN NOT NULL DEFAULT FALSE,
    notes            TEXT NOT NULL DEFAULT '',
    version          BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    confirmed_at     TIMESTAMPTZ,
    confirmed_by     TEXT,
    cancelled_at     TIMESTAMPTZ,
    cancelled_by     TEXT,
    cancel_reason    TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, slip_number),
    FOREIGN KEY (organization_id, supplier_id)
        REFERENCES zaiko.suppliers(organization_id, id),
    FOREIGN KEY (organization_id, confirmed_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, cancelled_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_purchase_slips_org_date
    ON zaiko.purchase_slips (organization_id, purchase_date DESC, slip_number);

CREATE TABLE zaiko.purchase_slip_lines (
    id                       TEXT PRIMARY KEY,
    organization_id          TEXT NOT NULL REFERENCES zaiko.organizations(id),
    purchase_slip_id         TEXT NOT NULL,
    line_number              INTEGER NOT NULL CHECK (line_number > 0),
    quantity                 INTEGER NOT NULL CHECK (quantity > 0),
    unit_cost_minor          BIGINT NOT NULL CHECK (unit_cost_minor >= 0),
    currency                 CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    base_sale_price_minor    BIGINT NOT NULL DEFAULT 0 CHECK (base_sale_price_minor >= 0),
    brand                    TEXT NOT NULL,
    model_number             TEXT NOT NULL DEFAULT '',
    product_type             TEXT NOT NULL,
    requested_product_code   TEXT NOT NULL DEFAULT '',
    sku                      TEXT NOT NULL DEFAULT '',
    serial_number            TEXT NOT NULL DEFAULT '',
    material_text            TEXT NOT NULL DEFAULT '',
    movement_text            TEXT NOT NULL DEFAULT '',
    condition_text           TEXT NOT NULL DEFAULT '',
    belt_material_text       TEXT NOT NULL DEFAULT '',
    dial_text                TEXT NOT NULL DEFAULT '',
    box_text                 TEXT NOT NULL DEFAULT '',
    accessories              TEXT NOT NULL DEFAULT '',
    features_text            TEXT NOT NULL DEFAULT '',
    generated_product_count  INTEGER NOT NULL DEFAULT 0 CHECK (generated_product_count >= 0),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, purchase_slip_id, line_number),
    FOREIGN KEY (organization_id, purchase_slip_id)
        REFERENCES zaiko.purchase_slips(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE zaiko.products (
    id                       TEXT PRIMARY KEY,
    organization_id          TEXT NOT NULL REFERENCES zaiko.organizations(id),
    product_code             TEXT NOT NULL,
    sku                      TEXT NOT NULL DEFAULT '',
    brand                    TEXT NOT NULL,
    model_number             TEXT NOT NULL DEFAULT '',
    serial_number            TEXT NOT NULL DEFAULT '',
    product_type             TEXT NOT NULL,
    purchase_slip_line_id    TEXT,
    supplier_id              TEXT NOT NULL,
    purchase_date            DATE NOT NULL,
    cost_amount_minor        BIGINT NOT NULL CHECK (cost_amount_minor >= 0),
    cost_currency            CHAR(3) NOT NULL CHECK (cost_currency ~ '^[A-Z]{3}$'),
    base_sale_price_minor    BIGINT NOT NULL DEFAULT 0 CHECK (base_sale_price_minor >= 0),
    base_sale_currency       CHAR(3) NOT NULL DEFAULT 'JPY' CHECK (base_sale_currency ~ '^[A-Z]{3}$'),
    inventory_status         TEXT NOT NULL CHECK (
                                 inventory_status IN (
                                     'purchasing', 'in_stock', 'reserved', 'sold',
                                     'shipped', 'cancelled', 'invalid'
                                 )
                             ),
    publication_status       TEXT NOT NULL DEFAULT 'private'
                             CHECK (publication_status IN ('private', 'public')),
    condition_text           TEXT NOT NULL DEFAULT '',
    accessories              TEXT NOT NULL DEFAULT '',
    material_text            TEXT NOT NULL DEFAULT '',
    box_text                 TEXT NOT NULL DEFAULT '',
    movement_text            TEXT NOT NULL DEFAULT '',
    belt_material_text       TEXT NOT NULL DEFAULT '',
    dial_text                TEXT NOT NULL DEFAULT '',
    features_text            TEXT NOT NULL DEFAULT '',
    internal_comment_text    TEXT NOT NULL DEFAULT '',
    version                  BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    cancelled_at             TIMESTAMPTZ,
    cancelled_by             TEXT,
    cancel_reason            TEXT NOT NULL DEFAULT '',
    deleted_at               TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, product_code),
    FOREIGN KEY (organization_id, purchase_slip_line_id)
        REFERENCES zaiko.purchase_slip_lines(organization_id, id),
    FOREIGN KEY (organization_id, supplier_id)
        REFERENCES zaiko.suppliers(organization_id, id),
    FOREIGN KEY (organization_id, cancelled_by)
        REFERENCES zaiko.users(organization_id, id),
    CHECK (btrim(product_code) <> ''),
    CHECK (btrim(brand) <> '')
);

CREATE INDEX idx_products_org_status
    ON zaiko.products (organization_id, inventory_status, purchase_date DESC, product_code)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_products_org_serial
    ON zaiko.products (organization_id, serial_number)
    WHERE deleted_at IS NULL AND serial_number <> '';
CREATE INDEX idx_products_org_sku
    ON zaiko.products (organization_id, sku)
    WHERE deleted_at IS NULL AND sku <> '';

CREATE TABLE zaiko.accessory_masters (
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    accessory_code   TEXT NOT NULL,
    name             TEXT NOT NULL,
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order       INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, accessory_code)
);

CREATE TABLE zaiko.product_accessories (
    organization_id  TEXT NOT NULL,
    product_id       TEXT NOT NULL,
    accessory_code   TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, product_id, accessory_code),
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, accessory_code)
        REFERENCES zaiko.accessory_masters(organization_id, accessory_code)
);

CREATE TABLE zaiko.product_objects (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    product_id       TEXT NOT NULL,
    original_name    TEXT NOT NULL,
    content_type     TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    size_bytes       BIGINT CHECK (size_bytes BETWEEN 1 AND 15728640),
    checksum_sha256  CHAR(64) CHECK (checksum_sha256 ~ '^[0-9a-f]{64}$'),
    sort_order       INTEGER NOT NULL DEFAULT 0,
    status           TEXT NOT NULL CHECK (status IN ('pending', 'ready', 'failed', 'deleted')),
    failure_code     TEXT NOT NULL DEFAULT '',
    storage_provider TEXT NOT NULL DEFAULT 's3',
    storage_bucket   TEXT NOT NULL,
    storage_key      TEXT NOT NULL,
    storage_version  TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ready_at         TIMESTAMPTZ,
    deleted_at       TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    UNIQUE (storage_provider, storage_bucket, storage_key),
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id),
    CHECK (
        (status = 'ready' AND size_bytes IS NOT NULL AND checksum_sha256 IS NOT NULL AND ready_at IS NOT NULL)
        OR status <> 'ready'
    )
);

CREATE INDEX idx_product_objects_product
    ON zaiko.product_objects (organization_id, product_id, status, sort_order, id);
CREATE INDEX idx_product_objects_reconcile
    ON zaiko.product_objects (status, created_at)
    WHERE status IN ('pending', 'failed', 'deleted');

CREATE TABLE zaiko.inventory_events (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    product_id       TEXT NOT NULL,
    event_type       TEXT NOT NULL,
    from_status      TEXT NOT NULL DEFAULT '',
    to_status        TEXT NOT NULL DEFAULT '',
    reason           TEXT NOT NULL DEFAULT '',
    actor_user_id    TEXT NOT NULL,
    request_id       TEXT NOT NULL,
    idempotency_key  TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id),
    FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_inventory_events_product
    ON zaiko.inventory_events (organization_id, product_id, created_at DESC, id DESC);

CREATE TABLE zaiko.serial_duplicate_overrides (
    id                          TEXT PRIMARY KEY,
    organization_id             TEXT NOT NULL REFERENCES zaiko.organizations(id),
    product_id                  TEXT NOT NULL,
    serial_number               TEXT NOT NULL,
    candidate_product_ids_json  JSONB NOT NULL,
    reason                      TEXT NOT NULL,
    actor_user_id               TEXT NOT NULL,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id),
    FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE TABLE zaiko.market_import_batches (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    file_name        TEXT NOT NULL,
    status           TEXT NOT NULL CHECK (
                         status IN ('previewed', 'pending_approval', 'committed', 'rejected')
                     ),
    total_rows       INTEGER NOT NULL DEFAULT 0 CHECK (total_rows >= 0),
    valid_rows       INTEGER NOT NULL DEFAULT 0 CHECK (valid_rows >= 0),
    error_rows       INTEGER NOT NULL DEFAULT 0 CHECK (error_rows >= 0),
    duplicate_rows   INTEGER NOT NULL DEFAULT 0 CHECK (duplicate_rows >= 0),
    created_by       TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    committed_by     TEXT,
    committed_at     TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, committed_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE TABLE zaiko.market_price_records (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    market_date      DATE NOT NULL,
    brand            TEXT NOT NULL,
    model_number     TEXT NOT NULL DEFAULT '',
    product_type     TEXT NOT NULL,
    price_minor      BIGINT NOT NULL CHECK (price_minor >= 0),
    currency         CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    source           TEXT NOT NULL DEFAULT 'manual',
    notes            TEXT NOT NULL DEFAULT '',
    import_batch_id  TEXT,
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_by       TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, import_batch_id)
        REFERENCES zaiko.market_import_batches(organization_id, id),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_market_prices_lookup
    ON zaiko.market_price_records
       (organization_id, brand, model_number, product_type, market_date DESC)
    WHERE is_active;

CREATE TABLE zaiko.market_import_rows (
    id                      TEXT PRIMARY KEY,
    organization_id         TEXT NOT NULL REFERENCES zaiko.organizations(id),
    batch_id                TEXT NOT NULL,
    row_number              INTEGER NOT NULL CHECK (row_number > 0),
    market_date             DATE,
    brand                   TEXT NOT NULL DEFAULT '',
    model_number            TEXT NOT NULL DEFAULT '',
    product_type            TEXT NOT NULL DEFAULT '',
    price_minor             BIGINT NOT NULL DEFAULT 0 CHECK (price_minor >= 0),
    currency                CHAR(3) CHECK (currency ~ '^[A-Z]{3}$'),
    source                  TEXT NOT NULL DEFAULT '',
    raw_json                JSONB NOT NULL,
    is_valid                BOOLEAN NOT NULL DEFAULT FALSE,
    error_message           TEXT NOT NULL DEFAULT '',
    duplicate_candidate_id  TEXT,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, batch_id, row_number),
    FOREIGN KEY (organization_id, batch_id)
        REFERENCES zaiko.market_import_batches(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, duplicate_candidate_id)
        REFERENCES zaiko.market_price_records(organization_id, id)
);

CREATE TABLE zaiko.product_market_prices (
    organization_id            TEXT NOT NULL,
    product_id                  TEXT NOT NULL,
    purchase_market_price_minor BIGINT NOT NULL DEFAULT 0 CHECK (purchase_market_price_minor >= 0),
    sale_market_price_minor     BIGINT NOT NULL DEFAULT 0 CHECK (sale_market_price_minor >= 0),
    currency                    CHAR(3) NOT NULL DEFAULT 'JPY' CHECK (currency ~ '^[A-Z]{3}$'),
    version                     BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by                  TEXT NOT NULL,
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, product_id),
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, updated_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE TABLE zaiko.sales_slips (
    id                         TEXT PRIMARY KEY,
    organization_id            TEXT NOT NULL REFERENCES zaiko.organizations(id),
    slip_number                TEXT NOT NULL,
    sales_date                 DATE NOT NULL,
    customer_record_id         TEXT,
    customer_name              TEXT NOT NULL,
    customer_address           TEXT NOT NULL DEFAULT '',
    customer_phone             TEXT NOT NULL DEFAULT '',
    qualified_invoice_number   TEXT NOT NULL DEFAULT '',
    status                     TEXT NOT NULL CHECK (
                                   status IN ('draft', 'pending_approval', 'confirmed', 'cancelled')
                               ),
    tax_mode                   TEXT NOT NULL DEFAULT 'standard'
                               CHECK (tax_mode IN ('standard', 'exempt')),
    settlement_currency        CHAR(3) NOT NULL DEFAULT 'JPY'
                               CHECK (settlement_currency ~ '^[A-Z]{3}$'),
    fx_rate_scaled             BIGINT NOT NULL DEFAULT 1 CHECK (fx_rate_scaled > 0),
    fx_rate_scale              BIGINT NOT NULL DEFAULT 1 CHECK (fx_rate_scale > 0),
    subtotal_minor             BIGINT NOT NULL DEFAULT 0 CHECK (subtotal_minor >= 0),
    tax_minor                  BIGINT NOT NULL DEFAULT 0 CHECK (tax_minor >= 0),
    total_minor                BIGINT NOT NULL DEFAULT 0 CHECK (total_minor >= 0),
    notes                      TEXT NOT NULL DEFAULT '',
    version                    BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    confirmed_at               TIMESTAMPTZ,
    confirmed_by               TEXT,
    cancelled_at               TIMESTAMPTZ,
    cancelled_by               TEXT,
    cancel_reason              TEXT NOT NULL DEFAULT '',
    created_by                 TEXT NOT NULL,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, slip_number),
    FOREIGN KEY (organization_id, customer_record_id)
        REFERENCES zaiko.master_records(organization_id, id),
    FOREIGN KEY (organization_id, confirmed_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, cancelled_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_sales_slips_org_date
    ON zaiko.sales_slips (organization_id, sales_date DESC, slip_number);

CREATE TABLE zaiko.sales_lines (
    id                         TEXT PRIMARY KEY,
    organization_id            TEXT NOT NULL REFERENCES zaiko.organizations(id),
    sales_slip_id              TEXT NOT NULL,
    line_number                INTEGER NOT NULL CHECK (line_number > 0),
    product_id                 TEXT NOT NULL,
    quantity                   INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_minor           BIGINT NOT NULL CHECK (unit_price_minor >= 0),
    sale_currency              CHAR(3) NOT NULL CHECK (sale_currency ~ '^[A-Z]{3}$'),
    exchange_rate_snapshot_id  TEXT,
    exchange_rate_scaled       BIGINT NOT NULL DEFAULT 1 CHECK (exchange_rate_scaled > 0),
    exchange_rate_scale        BIGINT NOT NULL DEFAULT 1 CHECK (exchange_rate_scale > 0),
    exchange_rate_observed_at  TIMESTAMPTZ,
    converted_unit_price_jpy   BIGINT NOT NULL DEFAULT 0 CHECK (converted_unit_price_jpy >= 0),
    converted_total_jpy        BIGINT NOT NULL DEFAULT 0 CHECK (converted_total_jpy >= 0),
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, sales_slip_id, line_number),
    UNIQUE (organization_id, sales_slip_id, product_id),
    FOREIGN KEY (organization_id, sales_slip_id)
        REFERENCES zaiko.sales_slips(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id),
    FOREIGN KEY (organization_id, exchange_rate_snapshot_id)
        REFERENCES zaiko.exchange_rate_snapshots(organization_id, id)
);

CREATE INDEX idx_sales_lines_product
    ON zaiko.sales_lines (organization_id, product_id);

CREATE TABLE zaiko.sales_slip_revisions (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    sales_slip_id    TEXT NOT NULL,
    actor_user_id    TEXT NOT NULL,
    memo             TEXT NOT NULL,
    before_json      JSONB NOT NULL,
    after_json       JSONB NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id, sales_slip_id)
        REFERENCES zaiko.sales_slips(organization_id, id),
    FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_sales_slip_revisions_slip
    ON zaiko.sales_slip_revisions
       (organization_id, sales_slip_id, created_at DESC, id DESC);

CREATE TABLE zaiko.shipment_slips (
    id                     TEXT PRIMARY KEY,
    organization_id        TEXT NOT NULL REFERENCES zaiko.organizations(id),
    shipment_number        TEXT NOT NULL,
    sales_slip_id          TEXT,
    shipment_date          DATE NOT NULL,
    destination_record_id  TEXT,
    recipient_name         TEXT NOT NULL,
    recipient_address      TEXT NOT NULL DEFAULT '',
    recipient_phone        TEXT NOT NULL DEFAULT '',
    tracking_number        TEXT NOT NULL DEFAULT '',
    status                 TEXT NOT NULL CHECK (status IN ('draft', 'confirmed', 'cancelled')),
    notes                  TEXT NOT NULL DEFAULT '',
    version                BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    confirmed_at           TIMESTAMPTZ,
    confirmed_by           TEXT,
    cancelled_at           TIMESTAMPTZ,
    cancelled_by           TEXT,
    cancel_reason          TEXT NOT NULL DEFAULT '',
    created_by             TEXT NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, shipment_number),
    FOREIGN KEY (organization_id, sales_slip_id)
        REFERENCES zaiko.sales_slips(organization_id, id),
    FOREIGN KEY (organization_id, destination_record_id)
        REFERENCES zaiko.master_records(organization_id, id),
    FOREIGN KEY (organization_id, confirmed_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, cancelled_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_shipments_org_date
    ON zaiko.shipment_slips (organization_id, shipment_date DESC, shipment_number);

CREATE TABLE zaiko.shipment_lines (
    id                     TEXT PRIMARY KEY,
    organization_id        TEXT NOT NULL REFERENCES zaiko.organizations(id),
    shipment_slip_id       TEXT NOT NULL,
    line_number            INTEGER NOT NULL CHECK (line_number > 0),
    product_id             TEXT NOT NULL,
    quantity               INTEGER NOT NULL CHECK (quantity > 0),
    wholesale_price_minor  BIGINT NOT NULL DEFAULT 0 CHECK (wholesale_price_minor >= 0),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, shipment_slip_id, line_number),
    UNIQUE (organization_id, shipment_slip_id, product_id),
    FOREIGN KEY (organization_id, shipment_slip_id)
        REFERENCES zaiko.shipment_slips(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id)
);

CREATE TABLE zaiko.sales_shipment_allocations (
    id                  TEXT PRIMARY KEY,
    organization_id     TEXT NOT NULL REFERENCES zaiko.organizations(id),
    sales_line_id       TEXT NOT NULL,
    shipment_line_id    TEXT NOT NULL,
    allocated_quantity  INTEGER NOT NULL CHECK (allocated_quantity > 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, sales_line_id, shipment_line_id),
    FOREIGN KEY (organization_id, sales_line_id)
        REFERENCES zaiko.sales_lines(organization_id, id),
    FOREIGN KEY (organization_id, shipment_line_id)
        REFERENCES zaiko.shipment_lines(organization_id, id)
);

CREATE INDEX idx_allocations_sales_line
    ON zaiko.sales_shipment_allocations (organization_id, sales_line_id);

CREATE TABLE zaiko.shipment_slip_revisions (
    id                TEXT PRIMARY KEY,
    organization_id   TEXT NOT NULL REFERENCES zaiko.organizations(id),
    shipment_slip_id  TEXT NOT NULL,
    actor_user_id     TEXT NOT NULL,
    memo              TEXT NOT NULL,
    before_json       JSONB NOT NULL,
    after_json        JSONB NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id, shipment_slip_id)
        REFERENCES zaiko.shipment_slips(organization_id, id),
    FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_shipment_slip_revisions_slip
    ON zaiko.shipment_slip_revisions
       (organization_id, shipment_slip_id, created_at DESC, id DESC);

CREATE TABLE zaiko.return_takehome_items (
    id                     TEXT PRIMARY KEY,
    organization_id        TEXT NOT NULL REFERENCES zaiko.organizations(id),
    sales_slip_id          TEXT NOT NULL,
    sales_line_id          TEXT NOT NULL,
    action_type            TEXT NOT NULL CHECK (action_type IN ('return', 'take_home')),
    status                 TEXT NOT NULL DEFAULT 'pending'
                           CHECK (status IN ('pending', 'completed', 'cancelled')),
    quantity               INTEGER NOT NULL CHECK (quantity > 0),
    reason                 TEXT NOT NULL DEFAULT '',
    notes                  TEXT NOT NULL DEFAULT '',
    return_date            DATE,
    invoice_issued_at      TIMESTAMPTZ,
    invoice_issued_by      TEXT,
    invoice_printed_at     TIMESTAMPTZ,
    invoice_printed_by     TEXT,
    inventory_restored_at  TIMESTAMPTZ,
    inventory_restored_by  TEXT,
    restore_box_text       TEXT NOT NULL DEFAULT '',
    restore_comment_text   TEXT NOT NULL DEFAULT '',
    version                BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    requested_by           TEXT NOT NULL,
    requested_at           TIMESTAMPTZ NOT NULL,
    processed_by           TEXT,
    processed_at           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, sales_slip_id)
        REFERENCES zaiko.sales_slips(organization_id, id),
    FOREIGN KEY (organization_id, sales_line_id)
        REFERENCES zaiko.sales_lines(organization_id, id),
    FOREIGN KEY (organization_id, requested_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, processed_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, invoice_issued_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, invoice_printed_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, inventory_restored_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE UNIQUE INDEX ux_return_takehome_one_pending_action
    ON zaiko.return_takehome_items (organization_id, sales_line_id, action_type)
    WHERE status = 'pending';
CREATE INDEX idx_return_takehome_org_status
    ON zaiko.return_takehome_items (organization_id, status, requested_at DESC);
CREATE INDEX idx_return_takehome_restore
    ON zaiko.return_takehome_items (organization_id, inventory_restored_at, sales_slip_id);

CREATE TABLE zaiko.purchase_return_slips (
    id                       TEXT PRIMARY KEY,
    organization_id          TEXT NOT NULL REFERENCES zaiko.organizations(id),
    purchase_slip_id         TEXT,
    return_number            TEXT NOT NULL,
    return_date              DATE NOT NULL,
    supplier_id              TEXT,
    supplier_name            TEXT NOT NULL,
    item_count               INTEGER NOT NULL CHECK (item_count > 0),
    amount_minor             BIGINT NOT NULL CHECK (amount_minor >= 0),
    currency                 CHAR(3) NOT NULL DEFAULT 'JPY' CHECK (currency ~ '^[A-Z]{3}$'),
    reason                   TEXT NOT NULL DEFAULT '',
    notes                    TEXT NOT NULL DEFAULT '',
    status                   TEXT NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'returned', 'completed')),
    delivery_number          TEXT NOT NULL DEFAULT '',
    invoice_issued_at        TIMESTAMPTZ,
    invoice_issued_by        TEXT,
    invoice_printed_at       TIMESTAMPTZ,
    invoice_printed_by       TEXT,
    version                  BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by               TEXT,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, return_number),
    FOREIGN KEY (organization_id, purchase_slip_id)
        REFERENCES zaiko.purchase_slips(organization_id, id),
    FOREIGN KEY (organization_id, supplier_id)
        REFERENCES zaiko.suppliers(organization_id, id),
    FOREIGN KEY (organization_id, invoice_issued_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, invoice_printed_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_purchase_returns_org_date
    ON zaiko.purchase_return_slips (organization_id, return_date DESC, return_number);

CREATE TABLE zaiko.purchase_return_lines (
    id                       TEXT PRIMARY KEY,
    organization_id          TEXT NOT NULL REFERENCES zaiko.organizations(id),
    purchase_return_slip_id  TEXT NOT NULL,
    product_id               TEXT NOT NULL,
    product_code             TEXT NOT NULL,
    sku                      TEXT NOT NULL DEFAULT '',
    brand                    TEXT NOT NULL DEFAULT '',
    model_number             TEXT NOT NULL DEFAULT '',
    amount_minor             BIGINT NOT NULL CHECK (amount_minor >= 0),
    currency                 CHAR(3) NOT NULL DEFAULT 'JPY' CHECK (currency ~ '^[A-Z]{3}$'),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, purchase_return_slip_id, product_id),
    FOREIGN KEY (organization_id, purchase_return_slip_id)
        REFERENCES zaiko.purchase_return_slips(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id)
);

CREATE TABLE zaiko.purchase_slip_revisions (
    id                TEXT PRIMARY KEY,
    organization_id   TEXT NOT NULL REFERENCES zaiko.organizations(id),
    purchase_slip_id  TEXT NOT NULL,
    actor_user_id     TEXT NOT NULL,
    memo              TEXT NOT NULL,
    before_json       JSONB NOT NULL,
    after_json        JSONB NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id, purchase_slip_id)
        REFERENCES zaiko.purchase_slips(organization_id, id),
    FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_purchase_slip_revisions_slip
    ON zaiko.purchase_slip_revisions
       (organization_id, purchase_slip_id, created_at DESC, id DESC);

CREATE TABLE zaiko.purchase_requests (
    id                TEXT PRIMARY KEY,
    organization_id   TEXT NOT NULL REFERENCES zaiko.organizations(id),
    request_group_id  TEXT NOT NULL,
    product_id        TEXT NOT NULL,
    guest_company_id  TEXT,
    request_number    TEXT NOT NULL,
    guest_name        TEXT NOT NULL,
    guest_email       TEXT NOT NULL,
    guest_phone       TEXT NOT NULL DEFAULT '',
    message           TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL CHECK (
                          status IN ('pending', 'approved', 'rejected', 'cancelled', 'expired', 'sold')
                      ),
    version           BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    requested_at      TIMESTAMPTZ NOT NULL,
    reviewed_at       TIMESTAMPTZ,
    reviewed_by       TEXT,
    rejection_reason  TEXT NOT NULL DEFAULT '',
    cancelled_at      TIMESTAMPTZ,
    cancel_reason     TEXT NOT NULL DEFAULT '',
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, request_number),
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id),
    FOREIGN KEY (organization_id, reviewed_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_purchase_requests_org_group_status
    ON zaiko.purchase_requests
       (organization_id, request_group_id, status, requested_at DESC, id);
CREATE INDEX idx_purchase_requests_product
    ON zaiko.purchase_requests (organization_id, product_id, requested_at DESC);

CREATE TABLE zaiko.reservations (
    id                   TEXT PRIMARY KEY,
    organization_id      TEXT NOT NULL REFERENCES zaiko.organizations(id),
    product_id           TEXT NOT NULL,
    purchase_request_id  TEXT NOT NULL,
    status               TEXT NOT NULL CHECK (status IN ('active', 'expired', 'released', 'fulfilled')),
    starts_at            TIMESTAMPTZ NOT NULL,
    expires_at           TIMESTAMPTZ NOT NULL,
    released_at          TIMESTAMPTZ,
    release_reason       TEXT NOT NULL DEFAULT '',
    version              BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by           TEXT NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, purchase_request_id),
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id),
    FOREIGN KEY (organization_id, purchase_request_id)
        REFERENCES zaiko.purchase_requests(organization_id, id),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id),
    CHECK (expires_at > starts_at)
);

CREATE UNIQUE INDEX ux_active_reservation_product
    ON zaiko.reservations (organization_id, product_id)
    WHERE status = 'active';
CREATE INDEX idx_reservations_expiry
    ON zaiko.reservations (organization_id, expires_at)
    WHERE status = 'active';

CREATE TABLE zaiko.approval_requests (
    id                       TEXT PRIMARY KEY,
    organization_id          TEXT NOT NULL REFERENCES zaiko.organizations(id),
    approval_type            TEXT NOT NULL,
    target_type              TEXT NOT NULL,
    target_id                TEXT NOT NULL,
    action_key               TEXT NOT NULL,
    applicant_user_id        TEXT NOT NULL,
    status                   TEXT NOT NULL CHECK (
                                 status IN (
                                     'pending', 'approved', 'returned',
                                     'rejected', 'cancelled', 'expired'
                                 )
                             ),
    requested_snapshot       JSONB NOT NULL,
    requested_snapshot_hash  CHAR(64) NOT NULL CHECK (requested_snapshot_hash ~ '^[0-9a-f]{64}$'),
    request_reason           TEXT NOT NULL DEFAULT '',
    action_payload_json      JSONB NOT NULL DEFAULT '{}'::jsonb,
    version                  BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    requested_at             TIMESTAMPTZ NOT NULL,
    expires_at               TIMESTAMPTZ,
    decided_at               TIMESTAMPTZ,
    decided_by               TEXT,
    executed_at              TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, applicant_user_id)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, decided_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE UNIQUE INDEX ux_pending_approval_target
    ON zaiko.approval_requests (organization_id, target_type, target_id, action_key)
    WHERE status = 'pending';
CREATE INDEX idx_approvals_org_status
    ON zaiko.approval_requests (organization_id, status, requested_at DESC, id);

CREATE TABLE zaiko.approval_actions (
    id                   TEXT PRIMARY KEY,
    organization_id      TEXT NOT NULL REFERENCES zaiko.organizations(id),
    approval_request_id  TEXT NOT NULL,
    actor_user_id        TEXT NOT NULL,
    action               TEXT NOT NULL CHECK (
                             action IN ('requested', 'approved', 'returned', 'rejected', 'cancelled', 'expired')
                         ),
    comment              TEXT NOT NULL DEFAULT '',
    acted_at             TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (organization_id, approval_request_id)
        REFERENCES zaiko.approval_requests(organization_id, id),
    FOREIGN KEY (organization_id, actor_user_id)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_approval_actions_request
    ON zaiko.approval_actions
       (organization_id, approval_request_id, acted_at, id);

CREATE TABLE zaiko.stocktakes (
    id                    TEXT PRIMARY KEY,
    organization_id       TEXT NOT NULL REFERENCES zaiko.organizations(id),
    stocktake_number      TEXT NOT NULL,
    stocktake_date        DATE NOT NULL,
    status                TEXT NOT NULL CHECK (status IN ('draft', 'completed')),
    expected_count        INTEGER NOT NULL DEFAULT 0 CHECK (expected_count >= 0),
    expected_total_minor  BIGINT NOT NULL DEFAULT 0 CHECK (expected_total_minor >= 0),
    notes                 TEXT NOT NULL DEFAULT '',
    version               BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by            TEXT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    saved_at              TIMESTAMPTZ,
    completed_by          TEXT,
    completed_at          TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, stocktake_number),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, completed_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE UNIQUE INDEX ux_stocktakes_one_draft_per_org
    ON zaiko.stocktakes (organization_id)
    WHERE status = 'draft';
CREATE INDEX idx_stocktakes_org_date
    ON zaiko.stocktakes (organization_id, stocktake_date DESC, stocktake_number);

CREATE TABLE zaiko.stocktake_lines (
    id                 TEXT PRIMARY KEY,
    organization_id    TEXT NOT NULL REFERENCES zaiko.organizations(id),
    stocktake_id       TEXT NOT NULL,
    product_id         TEXT NOT NULL,
    expected_present   BOOLEAN NOT NULL DEFAULT TRUE,
    counted_present    BOOLEAN,
    difference_reason  TEXT NOT NULL DEFAULT '',
    review_status      TEXT NOT NULL DEFAULT 'none'
                       CHECK (review_status IN ('none', 'pending', 'approved')),
    notes              TEXT NOT NULL DEFAULT '',
    counted_by         TEXT,
    counted_at         TIMESTAMPTZ,
    finalized_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, stocktake_id, product_id),
    FOREIGN KEY (organization_id, stocktake_id)
        REFERENCES zaiko.stocktakes(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id),
    FOREIGN KEY (organization_id, counted_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_stocktake_lines_review
    ON zaiko.stocktake_lines
       (organization_id, stocktake_id, review_status, counted_present);

CREATE TABLE zaiko.guest_companies (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    company_code     TEXT NOT NULL,
    name             TEXT NOT NULL,
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    version          BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by       TEXT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, company_code),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES zaiko.users(organization_id, id),
    FOREIGN KEY (organization_id, updated_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_guest_companies_org_active
    ON zaiko.guest_companies (organization_id, is_active, company_code);

ALTER TABLE zaiko.purchase_requests
    ADD CONSTRAINT fk_purchase_requests_guest_company
    FOREIGN KEY (organization_id, guest_company_id)
    REFERENCES zaiko.guest_companies(organization_id, id);

CREATE TABLE zaiko.guest_credentials (
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    company_id       TEXT NOT NULL,
    guest_id         TEXT NOT NULL,
    email            TEXT NOT NULL,
    password_hash    TEXT NOT NULL,
    version          BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by       TEXT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, company_id),
    UNIQUE (organization_id, guest_id),
    FOREIGN KEY (organization_id, company_id)
        REFERENCES zaiko.guest_companies(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, updated_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE TABLE zaiko.guest_boxes (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES zaiko.organizations(id),
    box_number       INTEGER NOT NULL CHECK (box_number BETWEEN 1 AND 10),
    box_name         TEXT NOT NULL DEFAULT '',
    version          BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by       TEXT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, box_number),
    FOREIGN KEY (organization_id, updated_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE TABLE zaiko.guest_box_drafts (
    organization_id  TEXT NOT NULL,
    company_id       TEXT NOT NULL,
    box_id           TEXT NOT NULL,
    is_selected      BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by       TEXT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, company_id, box_id),
    FOREIGN KEY (organization_id, company_id)
        REFERENCES zaiko.guest_companies(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, box_id)
        REFERENCES zaiko.guest_boxes(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, updated_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE TABLE zaiko.guest_box_publications (
    organization_id  TEXT NOT NULL,
    company_id       TEXT NOT NULL,
    box_id           TEXT NOT NULL,
    is_published     BOOLEAN NOT NULL DEFAULT FALSE,
    publication_id   TEXT NOT NULL DEFAULT '',
    published_by     TEXT,
    published_at     TIMESTAMPTZ,
    PRIMARY KEY (organization_id, company_id, box_id),
    FOREIGN KEY (organization_id, company_id)
        REFERENCES zaiko.guest_companies(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, box_id)
        REFERENCES zaiko.guest_boxes(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, published_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE TABLE zaiko.guest_box_products (
    organization_id  TEXT NOT NULL,
    box_id           TEXT NOT NULL,
    product_id       TEXT NOT NULL,
    sort_order       INTEGER NOT NULL DEFAULT 0,
    added_by         TEXT,
    added_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (organization_id, box_id, product_id),
    FOREIGN KEY (organization_id, box_id)
        REFERENCES zaiko.guest_boxes(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id),
    FOREIGN KEY (organization_id, added_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_guest_box_products_box
    ON zaiko.guest_box_products (organization_id, box_id, sort_order, product_id);

CREATE TABLE zaiko.guest_box_published_products (
    organization_id    TEXT NOT NULL,
    company_id         TEXT NOT NULL,
    box_id             TEXT NOT NULL,
    product_id         TEXT NOT NULL,
    publication_id     TEXT NOT NULL,
    sort_order         INTEGER NOT NULL DEFAULT 0,
    product_code       TEXT NOT NULL,
    brand              TEXT NOT NULL,
    model_name         TEXT NOT NULL DEFAULT '',
    reference_number   TEXT NOT NULL DEFAULT '',
    serial_number      TEXT NOT NULL DEFAULT '',
    sale_price_minor   BIGINT NOT NULL DEFAULT 0 CHECK (sale_price_minor >= 0),
    sale_currency      CHAR(3) NOT NULL DEFAULT 'JPY' CHECK (sale_currency ~ '^[A-Z]{3}$'),
    condition_text     TEXT NOT NULL DEFAULT '',
    accessories        TEXT NOT NULL DEFAULT '',
    inventory_status   TEXT NOT NULL,
    publication_status TEXT NOT NULL,
    box_name           TEXT NOT NULL DEFAULT '',
    published_by       TEXT,
    published_at       TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (organization_id, company_id, box_id, product_id, publication_id),
    FOREIGN KEY (organization_id, company_id)
        REFERENCES zaiko.guest_companies(organization_id, id),
    FOREIGN KEY (organization_id, box_id)
        REFERENCES zaiko.guest_boxes(organization_id, id),
    FOREIGN KEY (organization_id, product_id)
        REFERENCES zaiko.products(organization_id, id),
    FOREIGN KEY (organization_id, published_by)
        REFERENCES zaiko.users(organization_id, id)
);

CREATE INDEX idx_guest_box_published_products_lookup
    ON zaiko.guest_box_published_products
       (organization_id, company_id, box_id, publication_id, sort_order, product_id);

CREATE TABLE zaiko.guest_box_published_images (
    organization_id  TEXT NOT NULL,
    company_id       TEXT NOT NULL,
    box_id           TEXT NOT NULL,
    product_id       TEXT NOT NULL,
    publication_id   TEXT NOT NULL,
    image_id         TEXT NOT NULL,
    storage_path     TEXT NOT NULL,
    original_name    TEXT NOT NULL,
    content_type     TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    size_bytes       BIGINT NOT NULL CHECK (size_bytes > 0),
    sort_order       INTEGER NOT NULL DEFAULT 0,
    published_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (
        organization_id, company_id, box_id, product_id, publication_id, image_id
    ),
    FOREIGN KEY (
        organization_id, company_id, box_id, product_id, publication_id
    ) REFERENCES zaiko.guest_box_published_products (
        organization_id, company_id, box_id, product_id, publication_id
    ) ON DELETE CASCADE
);

CREATE INDEX idx_guest_box_published_images_lookup
    ON zaiko.guest_box_published_images
       (organization_id, company_id, product_id, publication_id, sort_order, image_id);

INSERT INTO zaiko.schema_migrations (
    version,
    name,
    checksum_sha256,
    execution_id,
    execution_duration_ms
) VALUES (
    1,
    '000001_initial_schema',
    :'migration_checksum',
    :'execution_id',
    0
);

COMMIT;
