ALTER TABLE sales_slips ADD COLUMN IF NOT EXISTS paid_at TIMESTAMPTZ;
ALTER TABLE sales_slips ADD COLUMN IF NOT EXISTS paid_by TEXT REFERENCES users(id);

-- Existing slips before the latest sales date are treated as already paid.
WITH latest_dates AS (
    SELECT organization_id, MAX(sale_date) AS latest_sale_date
    FROM sales_slips
    GROUP BY organization_id
)
UPDATE sales_slips AS s
SET paid_at = COALESCE(s.issued_at, s.confirmed_at, s.created_at),
    paid_by = COALESCE(s.issued_by, s.confirmed_by, s.created_by),
    updated_at = GREATEST(s.updated_at, COALESCE(s.issued_at, s.confirmed_at, s.created_at))
FROM latest_dates AS d
WHERE s.organization_id = d.organization_id
  AND s.sale_date < d.latest_sale_date
  AND s.paid_at IS NULL;
