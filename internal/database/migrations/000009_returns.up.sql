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
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (sales_slip_id) REFERENCES sales_slips(id),
    FOREIGN KEY (sales_line_id) REFERENCES sales_lines(id),
    FOREIGN KEY (requested_by) REFERENCES users(id),
    FOREIGN KEY (processed_by) REFERENCES users(id)
);

CREATE INDEX idx_return_takehome_org_status
ON return_takehome_items(organization_id,status,requested_at DESC);

CREATE INDEX idx_return_takehome_sale
ON return_takehome_items(organization_id,sales_slip_id);

CREATE UNIQUE INDEX idx_return_takehome_one_pending_action
ON return_takehome_items(organization_id,sales_line_id,action_type)
WHERE status='pending';
