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
