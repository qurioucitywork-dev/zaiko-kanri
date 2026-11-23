ALTER TABLE purchase_slips
    DROP CONSTRAINT IF EXISTS purchase_slips_purchase_tax_mode_chk,
    DROP CONSTRAINT IF EXISTS purchase_slips_tax_rate_basis_points_chk,
    DROP COLUMN IF EXISTS tax_rate_basis_points,
    DROP COLUMN IF EXISTS purchase_tax_mode;
