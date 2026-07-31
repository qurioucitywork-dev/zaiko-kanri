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
