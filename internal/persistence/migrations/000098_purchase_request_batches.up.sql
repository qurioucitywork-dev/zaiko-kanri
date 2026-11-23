CREATE TABLE purchase_request_batches (
 id TEXT PRIMARY KEY,
 organization_id TEXT NOT NULL REFERENCES organizations(id),
 request_number TEXT NOT NULL,
 guest_account_id TEXT NOT NULL REFERENCES guest_accounts(id),
 buyer_role_id TEXT NOT NULL REFERENCES partner_roles(id),
 shipment_slip_id TEXT REFERENCES shipment_slips(id),
 requested_at TIMESTAMPTZ NOT NULL,
 reviewed_at TIMESTAMPTZ,
 reviewed_by TEXT REFERENCES users(id),
 UNIQUE(organization_id,request_number)
);
ALTER TABLE purchase_requests ADD COLUMN batch_id TEXT REFERENCES purchase_request_batches(id);
CREATE INDEX idx_purchase_requests_batch ON purchase_requests(batch_id);
