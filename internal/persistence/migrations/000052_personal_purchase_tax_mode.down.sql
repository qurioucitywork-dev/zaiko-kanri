UPDATE purchase_slips
SET purchase_tax_mode = 'domestic',
    tax_rate_basis_points = 1000,
    updated_at = CURRENT_TIMESTAMP
WHERE purchase_tax_mode = 'personal';

ALTER TABLE purchase_slips
    DROP CONSTRAINT IF EXISTS purchase_slips_purchase_tax_mode_chk,
    DROP CONSTRAINT IF EXISTS purchase_slips_tax_rate_basis_points_chk;

ALTER TABLE purchase_slips
    ADD CONSTRAINT purchase_slips_purchase_tax_mode_chk
        CHECK (purchase_tax_mode IN ('domestic', 'overseas')),
    ADD CONSTRAINT purchase_slips_tax_rate_basis_points_chk
        CHECK (
            (purchase_tax_mode = 'domestic' AND tax_rate_basis_points = 1000)
            OR (purchase_tax_mode = 'overseas' AND tax_rate_basis_points = 0)
        );

COMMENT ON COLUMN purchase_slips.purchase_tax_mode IS
    'Purchase tax category fixed when the slip is created: domestic or overseas.';
COMMENT ON COLUMN purchase_slips.tax_rate_basis_points IS
    'Tax rate snapshot in basis points. Domestic=1000 (10%), overseas=0.';
