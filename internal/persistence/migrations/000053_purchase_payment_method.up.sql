ALTER TABLE purchase_slips
    ADD COLUMN payment_method TEXT NOT NULL DEFAULT 'bank_transfer';

ALTER TABLE purchase_slips
    ADD CONSTRAINT purchase_slips_payment_method_check
    CHECK (payment_method IN ('cash', 'bank_transfer', 'card'));

COMMENT ON COLUMN purchase_slips.payment_method IS
    'Purchase payment method: cash, bank_transfer, or card';
