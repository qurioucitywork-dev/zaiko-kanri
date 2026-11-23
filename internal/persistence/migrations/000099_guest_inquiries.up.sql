CREATE TABLE guest_inquiries (
 id TEXT PRIMARY KEY,
 organization_id TEXT NOT NULL REFERENCES organizations(id),
 guest_user_id TEXT NOT NULL REFERENCES users(id),
 client_request_id TEXT NOT NULL,
 guest_code TEXT NOT NULL,
 guest_name TEXT NOT NULL,
 company_name TEXT NOT NULL,
 email TEXT NOT NULL DEFAULT '',
 subject TEXT NOT NULL,
 product_code TEXT NOT NULL DEFAULT '',
 message TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(organization_id, guest_user_id, client_request_id)
);
CREATE INDEX guest_inquiries_org_created ON guest_inquiries(organization_id, created_at DESC);
