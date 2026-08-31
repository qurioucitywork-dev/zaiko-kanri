UPDATE sales_slips
SET tax_mode = 'tax_exempt',
    tax_rate_basis_points = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE tax_mode = 'out_of_scope';

ALTER TABLE sales_slips
    DROP CONSTRAINT IF EXISTS sales_slips_tax_mode_chk;

ALTER TABLE sales_slips
    ADD CONSTRAINT sales_slips_tax_mode_check
        CHECK (tax_mode IN ('taxable', 'tax_exempt'));
