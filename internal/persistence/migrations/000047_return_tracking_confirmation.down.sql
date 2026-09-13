ALTER TABLE return_slips
    DROP COLUMN IF EXISTS tracking_confirmed_by,
    DROP COLUMN IF EXISTS tracking_confirmed_at;
