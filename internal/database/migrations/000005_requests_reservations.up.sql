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
