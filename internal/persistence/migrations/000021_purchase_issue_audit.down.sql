DROP INDEX IF EXISTS idx_purchase_slips_issued_at;

ALTER TABLE purchase_slips
    DROP COLUMN IF EXISTS issued_by,
    DROP COLUMN IF EXISTS issued_at;
