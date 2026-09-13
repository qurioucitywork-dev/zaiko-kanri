CREATE TABLE IF NOT EXISTS part_names (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_by TEXT REFERENCES users(id),
    updated_by TEXT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, code)
);

CREATE INDEX IF NOT EXISTS idx_part_names_org_active
    ON part_names (organization_id, is_active, sort_order, code);

CREATE TABLE IF NOT EXISTS part_code_sequences (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    business_date DATE NOT NULL,
    last_sequence INTEGER NOT NULL CHECK (last_sequence BETWEEN 0 AND 9999),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (organization_id, business_date)
);

CREATE TABLE IF NOT EXISTS parts (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    part_code TEXT NOT NULL,
    purchase_date DATE NOT NULL,
    purchase_staff_profile_id TEXT REFERENCES staff_profiles(id),
    supplier_role_id TEXT REFERENCES partner_roles(id),
    purchase_tax_mode TEXT NOT NULL DEFAULT 'domestic'
        CHECK (purchase_tax_mode IN ('domestic', 'personal', 'overseas')),
    tax_category TEXT NOT NULL DEFAULT 'consumption_tax'
        CHECK (tax_category IN ('consumption_tax', 'tax_equivalent', 'out_of_scope')),
    cost_amount_minor BIGINT NOT NULL DEFAULT 0 CHECK (cost_amount_minor >= 0),
    cost_currency TEXT NOT NULL DEFAULT 'JPY' CHECK (cost_currency IN ('JPY', 'USD', 'HKD')),
    fixed_cost_jpy_minor BIGINT NOT NULL DEFAULT 0 CHECK (fixed_cost_jpy_minor >= 0),
    fx_rate_id TEXT REFERENCES exchange_rate_snapshots(id),
    fx_rate_scaled BIGINT,
    fx_scale BIGINT,
    sku TEXT NOT NULL DEFAULT '',
    brand_id TEXT REFERENCES brands(id),
    brand_text TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    reference_number TEXT NOT NULL DEFAULT '',
    part_name_id TEXT NOT NULL REFERENCES part_names(id),
    part_name_text TEXT NOT NULL DEFAULT '',
    detail_text TEXT NOT NULL DEFAULT '',
    bracelet_quantity INTEGER CHECK (bracelet_quantity IS NULL OR bracelet_quantity >= 0),
    notes TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'in_stock' CHECK (status IN ('in_stock', 'cost_adjustment', 'invalid')),
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, part_code)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_parts_organization_part_code_normalized
    ON parts (organization_id, UPPER(BTRIM(part_code)));

CREATE INDEX IF NOT EXISTS idx_parts_org_purchase_date
    ON parts (organization_id, purchase_date DESC, part_code DESC);
