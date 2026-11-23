ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_org_email
    ON users (organization_id, LOWER(email))
    WHERE email <> '' AND deleted_at IS NULL;
