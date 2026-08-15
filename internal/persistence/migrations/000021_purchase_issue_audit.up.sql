ALTER TABLE purchase_slips
    ADD COLUMN IF NOT EXISTS issued_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS issued_by VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_purchase_slips_issued_at
    ON purchase_slips(organization_id, issued_at DESC)
    WHERE issued_at IS NOT NULL;
