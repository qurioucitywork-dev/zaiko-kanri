-- Confirmed cash purchases are settled at registration; retain any recorded payment.
UPDATE purchase_slips
SET paid_at=COALESCE(confirmed_at,created_at),
    paid_by=COALESCE(paid_by,confirmed_by,created_by),
    updated_at=now()
WHERE payment_method='cash' AND status='confirmed' AND paid_at IS NULL;
