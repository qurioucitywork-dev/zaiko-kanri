ALTER TABLE sales_slips ADD COLUMN customer_address TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_slips ADD COLUMN customer_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE sales_slips ADD COLUMN qualified_invoice_number TEXT NOT NULL DEFAULT '';

ALTER TABLE return_takehome_items ADD COLUMN return_date TEXT NOT NULL DEFAULT '';

CREATE TABLE sales_slip_revisions (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    sales_slip_id TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    memo TEXT NOT NULL,
    before_json TEXT NOT NULL,
    after_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (sales_slip_id) REFERENCES sales_slips(id),
    FOREIGN KEY (actor_user_id) REFERENCES users(id)
);

CREATE INDEX idx_sales_slip_revisions_slip
ON sales_slip_revisions(organization_id,sales_slip_id,created_at DESC);
