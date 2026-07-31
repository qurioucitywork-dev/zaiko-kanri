ALTER TABLE purchase_requests
    ADD COLUMN request_group_id TEXT NOT NULL DEFAULT '';

UPDATE purchase_requests
SET request_group_id = id
WHERE request_group_id = '';

CREATE INDEX IF NOT EXISTS idx_purchase_requests_org_group_status
    ON purchase_requests (organization_id, request_group_id, status, requested_at DESC);
