ALTER TABLE purchase_slips
    DROP CONSTRAINT IF EXISTS purchase_slips_purchase_tax_mode_chk,
    DROP CONSTRAINT IF EXISTS purchase_slips_tax_rate_basis_points_chk;

ALTER TABLE purchase_slips
    ADD CONSTRAINT purchase_slips_purchase_tax_mode_chk
        CHECK (purchase_tax_mode IN ('domestic', 'personal', 'overseas')),
    ADD CONSTRAINT purchase_slips_tax_rate_basis_points_chk
        CHECK (
            (purchase_tax_mode = 'domestic' AND tax_rate_basis_points = 1000)
            OR (purchase_tax_mode IN ('personal', 'overseas') AND tax_rate_basis_points = 0)
        );

COMMENT ON COLUMN purchase_slips.purchase_tax_mode IS
    'Purchase category fixed at creation: domestic business/auction, personal purchase, or overseas.';
COMMENT ON COLUMN purchase_slips.tax_rate_basis_points IS
    'Tax snapshot in basis points. Domestic business/auction=1000; personal and overseas=0.';
