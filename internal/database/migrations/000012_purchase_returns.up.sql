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
