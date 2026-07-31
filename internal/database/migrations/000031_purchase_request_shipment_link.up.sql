ALTER TABLE shipment_slips
    ADD COLUMN purchase_request_group_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_shipment_slips_org_purchase_request_group
    ON shipment_slips (organization_id, purchase_request_group_id)
    WHERE purchase_request_group_id <> '';
