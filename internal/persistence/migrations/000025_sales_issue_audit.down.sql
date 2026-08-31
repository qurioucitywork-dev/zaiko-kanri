DROP INDEX IF EXISTS idx_sales_slips_issued_at;

ALTER TABLE sales_slips
    DROP COLUMN IF EXISTS issued_by,
    DROP COLUMN IF EXISTS issued_at;
