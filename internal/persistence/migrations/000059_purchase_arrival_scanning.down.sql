-- Existing rows cannot be reliably distinguished from newly scanned rows.
-- Keep their current operational status when rolling back.
SELECT 1;
