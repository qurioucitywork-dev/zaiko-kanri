ALTER TABLE purchase_slips
    DROP CONSTRAINT IF EXISTS purchase_slips_payment_method_check;

ALTER TABLE purchase_slips
    DROP COLUMN IF EXISTS payment_method;
