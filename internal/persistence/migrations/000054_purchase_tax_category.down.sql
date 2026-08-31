ALTER TABLE purchase_slips
    DROP CONSTRAINT IF EXISTS purchase_slips_tax_category_check;

ALTER TABLE purchase_slips
    DROP COLUMN IF EXISTS tax_category;
