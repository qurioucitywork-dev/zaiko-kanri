ALTER TABLE purchase_slips
    ADD COLUMN tax_category TEXT NOT NULL DEFAULT 'consumption_tax';

UPDATE purchase_slips
SET tax_category = CASE
    WHEN purchase_tax_mode = 'domestic' THEN 'consumption_tax'
    ELSE 'out_of_scope'
END;

ALTER TABLE purchase_slips
    ADD CONSTRAINT purchase_slips_tax_category_check
    CHECK (tax_category IN ('consumption_tax', 'tax_equivalent', 'out_of_scope'));

COMMENT ON COLUMN purchase_slips.tax_category IS
    'Purchase tax handling: consumption_tax, tax_equivalent (internal reference only), or out_of_scope';
