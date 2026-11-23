CREATE TABLE IF NOT EXISTS publication_boxes (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    box_code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, box_code)
);

CREATE TABLE IF NOT EXISTS publication_box_buyers (
    box_id TEXT NOT NULL REFERENCES publication_boxes(id) ON DELETE CASCADE,
    buyer_role_id TEXT NOT NULL REFERENCES partner_roles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (box_id, buyer_role_id)
);

CREATE TABLE IF NOT EXISTS publication_box_products (
    box_id TEXT NOT NULL REFERENCES publication_boxes(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (box_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_publication_box_products_product
    ON publication_box_products (product_id, box_id);

CREATE TABLE IF NOT EXISTS purchase_request_sequences (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    business_year INTEGER NOT NULL CHECK (business_year BETWEEN 2000 AND 9999),
    last_sequence INTEGER NOT NULL CHECK (last_sequence > 0),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (organization_id, business_year)
);

CREATE TABLE IF NOT EXISTS purchase_requests (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    request_number TEXT NOT NULL,
    guest_account_id TEXT NOT NULL REFERENCES guest_accounts(id),
    buyer_role_id TEXT NOT NULL REFERENCES partner_roles(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled', 'expired', 'sold')),
    message TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL,
    reviewed_by TEXT REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    review_note TEXT NOT NULL DEFAULT '',
    reservation_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, request_number)
);

CREATE INDEX IF NOT EXISTS idx_purchase_requests_org_status
    ON purchase_requests (organization_id, status, requested_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uq_purchase_requests_active_reservation
    ON purchase_requests (organization_id, product_id)
    WHERE status = 'approved';

CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    recipient_user_id TEXT REFERENCES users(id),
    recipient_role_key TEXT REFERENCES roles(role_key),
    event_key TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    target_type TEXT NOT NULL DEFAULT '',
    target_id TEXT NOT NULL DEFAULT '',
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (recipient_user_id IS NOT NULL OR recipient_role_key IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_notifications_recipient_user
    ON notifications (organization_id, recipient_user_id, read_at, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notifications_recipient_role
    ON notifications (organization_id, recipient_role_key, read_at, created_at DESC);

CREATE TABLE IF NOT EXISTS approval_requests (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    approval_type TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    requested_action TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'returned', 'rejected', 'cancelled', 'expired')),
    requested_by TEXT NOT NULL REFERENCES users(id),
    requested_at TIMESTAMPTZ NOT NULL,
    decided_by TEXT REFERENCES users(id),
    decided_at TIMESTAMPTZ,
    decision_note TEXT NOT NULL DEFAULT '',
    snapshot_json JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_approval_target_pending
    ON approval_requests (organization_id, target_type, target_id, requested_action)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_approval_requests_pending
    ON approval_requests (organization_id, status, requested_at DESC);

CREATE TABLE IF NOT EXISTS approval_actions (
    id TEXT PRIMARY KEY,
    approval_request_id TEXT NOT NULL REFERENCES approval_requests(id),
    actor_user_id TEXT NOT NULL REFERENCES users(id),
    action TEXT NOT NULL CHECK (action IN ('requested', 'approved', 'returned', 'rejected', 'cancelled', 'expired')),
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_approval_actions_request
    ON approval_actions (approval_request_id, created_at);
