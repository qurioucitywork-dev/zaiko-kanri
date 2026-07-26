CREATE TABLE IF NOT EXISTS approval_requests (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    approval_type TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    action_key TEXT NOT NULL,
    applicant_user_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'returned', 'rejected', 'cancelled', 'expired')),
    requested_snapshot TEXT NOT NULL,
    requested_snapshot_hash TEXT NOT NULL,
    request_reason TEXT NOT NULL DEFAULT '',
    action_payload_json TEXT NOT NULL DEFAULT '{}',
    requested_at TEXT NOT NULL,
    expires_at TEXT,
    decided_at TEXT,
    decided_by TEXT REFERENCES users(id),
    executed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_pending_approval_target
    ON approval_requests (organization_id, target_type, target_id, action_key)
    WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_approvals_org_status
    ON approval_requests (organization_id, status, requested_at DESC);

CREATE TABLE IF NOT EXISTS approval_actions (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    approval_request_id TEXT NOT NULL REFERENCES approval_requests(id),
    actor_user_id TEXT NOT NULL REFERENCES users(id),
    action TEXT NOT NULL CHECK (action IN ('requested', 'approved', 'returned', 'rejected', 'cancelled', 'expired')),
    comment TEXT NOT NULL DEFAULT '',
    acted_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_approval_actions_request
    ON approval_actions (organization_id, approval_request_id, acted_at);
