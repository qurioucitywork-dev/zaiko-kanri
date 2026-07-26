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
