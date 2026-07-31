ALTER TABLE shipment_slips ADD COLUMN recipient_address TEXT NOT NULL DEFAULT '';
ALTER TABLE shipment_slips ADD COLUMN recipient_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE shipment_slips ADD COLUMN tracking_number TEXT NOT NULL DEFAULT '';

ALTER TABLE shipment_lines ADD COLUMN wholesale_price_minor INTEGER NOT NULL DEFAULT 0
    CHECK(wholesale_price_minor >= 0);

CREATE TABLE shipment_slip_revisions (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    shipment_slip_id TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    memo TEXT NOT NULL,
    before_json TEXT NOT NULL,
    after_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (shipment_slip_id) REFERENCES shipment_slips(id),
    FOREIGN KEY (actor_user_id) REFERENCES users(id)
);

CREATE INDEX idx_shipment_slip_revisions_slip
ON shipment_slip_revisions(organization_id,shipment_slip_id,created_at DESC);
