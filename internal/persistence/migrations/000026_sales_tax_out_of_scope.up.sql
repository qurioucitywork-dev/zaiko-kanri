ALTER TABLE sales_slips
    DROP CONSTRAINT IF EXISTS sales_slips_tax_mode_check,
    DROP CONSTRAINT IF EXISTS sales_slips_tax_mode_chk;

ALTER TABLE sales_slips
    ADD CONSTRAINT sales_slips_tax_mode_chk
        CHECK (tax_mode IN ('taxable', 'tax_exempt', 'out_of_scope'));

UPDATE sales_slips
SET tax_mode = 'out_of_scope',
    tax_rate_basis_points = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE display_currency = 'USD'
  AND tax_mode <> 'out_of_scope';

COMMENT ON COLUMN sales_slips.tax_mode IS
    'taxable=domestic 10%, tax_exempt=JPY exemption, out_of_scope=USD sale outside Japanese consumption-tax scope';
