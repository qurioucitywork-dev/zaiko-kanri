-- Provider-neutral object metadata used by the D1/R2 two-phase lifecycle.
-- Provider locators (bucket, object key, version, signed URL) are deliberately
-- absent from this schema.

-- SQLite requires the parent key of a composite foreign key to be covered by
-- an exact UNIQUE constraint. The baseline uses globally unique IDs, while
-- these additional keys let the database assert the tenant and ID together.
CREATE UNIQUE INDEX uq_products_organization_id_id
    ON products (organization_id, id);

CREATE UNIQUE INDEX uq_users_organization_id_id
    ON users (organization_id, id);

CREATE TABLE object_metadata (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    checksum_sha256 TEXT NOT NULL DEFAULT ''
        CHECK (
            checksum_sha256 = ''
            OR (
                length(checksum_sha256) = 64
                AND checksum_sha256 NOT GLOB '*[^0-9a-f]*'
            )
        ),
    original_name TEXT NOT NULL,
    content_type TEXT NOT NULL
        CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    size_bytes INTEGER NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'ready', 'failed', 'deleted')),
    failure_reason_code TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    ready_at TEXT,
    deleted_at TEXT,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, product_id)
        REFERENCES products (organization_id, id),
    FOREIGN KEY (organization_id, created_by)
        REFERENCES users (organization_id, id),
    CHECK (
        (status = 'pending' AND checksum_sha256 = '' AND size_bytes = 0
            AND ready_at IS NULL AND deleted_at IS NULL)
        OR
        (status = 'ready' AND checksum_sha256 <> '' AND size_bytes > 0
            AND ready_at IS NOT NULL AND deleted_at IS NULL)
        OR
        (status = 'failed' AND failure_reason_code <> ''
            AND ready_at IS NULL AND deleted_at IS NULL)
        OR
        (status = 'deleted' AND deleted_at IS NOT NULL)
    )
);

CREATE INDEX idx_object_metadata_product
    ON object_metadata (organization_id, product_id, status, sort_order, id);

CREATE INDEX idx_object_metadata_status
    ON object_metadata (organization_id, status, updated_at);

CREATE TABLE object_metadata_idempotency (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    operation_name TEXT NOT NULL
        CHECK (operation_name IN (
            'object.create_pending',
            'object.mark_ready',
            'object.mark_failed',
            'object.mark_deleted'
        )),
    idempotency_key TEXT NOT NULL CHECK (
        length(idempotency_key) BETWEEN 1 AND 128
    ),
    canonical_hash TEXT NOT NULL CHECK (
        length(canonical_hash) = 64
        AND canonical_hash NOT GLOB '*[^0-9a-f]*'
    ),
    object_id TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    response_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, operation_name, idempotency_key),
    FOREIGN KEY (organization_id, actor_id)
        REFERENCES users (organization_id, id)
);

CREATE INDEX idx_object_metadata_idempotency_object
    ON object_metadata_idempotency (organization_id, object_id, created_at);

-- Lifecycle transitions are one-way. Application statements still use an
-- explicit WHERE status = 'pending' predicate so they can distinguish a
-- transition conflict without exposing a cross-tenant row.
CREATE TRIGGER object_metadata_transition_guard
BEFORE UPDATE OF status ON object_metadata
WHEN OLD.status <> NEW.status
BEGIN
    SELECT CASE
        WHEN (
            OLD.status = 'pending'
            AND NEW.status IN ('ready', 'failed', 'deleted')
        )
        OR (
            OLD.status IN ('ready', 'failed')
            AND NEW.status = 'deleted'
        )
        THEN NULL
        ELSE RAISE(ABORT, 'invalid object metadata transition')
    END;
END;
