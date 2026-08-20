ALTER TABLE purchase_slips ADD COLUMN IF NOT EXISTS paid_at TIMESTAMPTZ;
ALTER TABLE purchase_slips ADD COLUMN IF NOT EXISTS paid_by TEXT REFERENCES users(id);

WITH ranked AS (
    SELECT id, ROW_NUMBER() OVER (
        PARTITION BY organization_id
        ORDER BY purchase_date DESC, slip_number DESC, created_at DESC
    ) AS position
    FROM purchase_slips
), historical AS (
    SELECT p.id,
           COALESCE(p.issued_at, p.confirmed_at, p.created_at) AS initial_paid_at,
           COALESCE(p.issued_by, p.confirmed_by, p.created_by) AS initial_paid_by
    FROM purchase_slips p
    JOIN ranked r ON r.id = p.id
    WHERE r.position > 3 AND p.paid_at IS NULL
)
UPDATE purchase_slips p
SET paid_at = h.initial_paid_at,
    paid_by = h.initial_paid_by,
    updated_at = GREATEST(p.updated_at, h.initial_paid_at)
FROM historical h
WHERE p.id = h.id;
