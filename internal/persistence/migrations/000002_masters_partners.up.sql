CREATE TABLE IF NOT EXISTS business_partners (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    partner_code TEXT NOT NULL,
    legal_name TEXT NOT NULL,
    representative_name TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    postal_code TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    invoice_number TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_by TEXT REFERENCES users(id),
    updated_by TEXT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, partner_code)
);

CREATE INDEX IF NOT EXISTS idx_business_partners_name
    ON business_partners (organization_id, legal_name);

CREATE TABLE IF NOT EXISTS partner_roles (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    partner_id TEXT NOT NULL REFERENCES business_partners(id),
    role_type TEXT NOT NULL CHECK (role_type IN ('buyer', 'supplier')),
    role_code TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    valid_from DATE,
    valid_to DATE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, role_type, role_code),
    UNIQUE (partner_id, role_type)
);

CREATE INDEX IF NOT EXISTS idx_partner_roles_partner
    ON partner_roles (organization_id, partner_id, role_type);

CREATE TABLE IF NOT EXISTS partner_contacts (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    partner_id TEXT NOT NULL REFERENCES business_partners(id),
    name TEXT NOT NULL,
    department TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_partner_primary_contact
    ON partner_contacts (partner_id)
    WHERE is_primary;

CREATE TABLE IF NOT EXISTS guest_accounts (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    guest_code TEXT NOT NULL,
    user_id TEXT NOT NULL UNIQUE REFERENCES users(id),
    buyer_role_id TEXT NOT NULL UNIQUE REFERENCES partner_roles(id),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('invited', 'active', 'suspended', 'closed')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, guest_code)
);

CREATE TABLE IF NOT EXISTS brands (
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

CREATE TABLE IF NOT EXISTS materials (
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

CREATE TABLE IF NOT EXISTS movements (
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

CREATE TABLE IF NOT EXISTS product_conditions (
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

CREATE TABLE IF NOT EXISTS accessories (
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

CREATE INDEX IF NOT EXISTS idx_brands_active ON brands (organization_id, is_active, sort_order);
CREATE INDEX IF NOT EXISTS idx_materials_active ON materials (organization_id, is_active, sort_order);
CREATE INDEX IF NOT EXISTS idx_movements_active ON movements (organization_id, is_active, sort_order);
CREATE INDEX IF NOT EXISTS idx_conditions_active ON product_conditions (organization_id, is_active, sort_order);
CREATE INDEX IF NOT EXISTS idx_accessories_active ON accessories (organization_id, is_active, sort_order);
