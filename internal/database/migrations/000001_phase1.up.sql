PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS permissions (
    permission_key TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS roles (
    role_key TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_key TEXT NOT NULL REFERENCES roles(role_key),
    permission_key TEXT NOT NULL REFERENCES permissions(permission_key),
    PRIMARY KEY (role_key, permission_key)
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role_key TEXT NOT NULL REFERENCES roles(role_key),
    is_active INTEGER NOT NULL DEFAULT 1,
    last_login_at TEXT,
    deleted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, username)
);

CREATE INDEX IF NOT EXISTS idx_users_org_active
    ON users (organization_id, is_active);

CREATE TABLE IF NOT EXISTS user_permissions (
    user_id TEXT NOT NULL REFERENCES users(id),
    permission_key TEXT NOT NULL REFERENCES permissions(permission_key),
    effect TEXT NOT NULL CHECK (effect IN ('allow', 'deny')),
    PRIMARY KEY (user_id, permission_key)
);

CREATE TABLE IF NOT EXISTS organization_settings (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    setting_key TEXT NOT NULL,
    setting_value TEXT NOT NULL,
    value_type TEXT NOT NULL DEFAULT 'string',
    is_configured INTEGER NOT NULL DEFAULT 0,
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, setting_key)
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    csrf_token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);

CREATE TABLE IF NOT EXISTS login_csrf_tokens (
    token_hash TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_login_csrf_expiry
    ON login_csrf_tokens (expires_at);

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    organization_id TEXT REFERENCES organizations(id),
    actor_user_id TEXT REFERENCES users(id),
    applicant_user_id TEXT REFERENCES users(id),
    approver_user_id TEXT REFERENCES users(id),
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    action TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '{}',
    after_json TEXT NOT NULL DEFAULT '{}',
    reason TEXT NOT NULL DEFAULT '',
    comment TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL,
    result TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_org_created
    ON audit_logs (organization_id, created_at DESC);

CREATE TRIGGER IF NOT EXISTS audit_logs_no_update
BEFORE UPDATE ON audit_logs
BEGIN
    SELECT RAISE(ABORT, 'audit logs are immutable');
END;

CREATE TRIGGER IF NOT EXISTS audit_logs_no_delete
BEFORE DELETE ON audit_logs
BEGIN
    SELECT RAISE(ABORT, 'audit logs are immutable');
END;
