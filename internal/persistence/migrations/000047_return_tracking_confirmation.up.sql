ALTER TABLE return_slips
    ADD COLUMN IF NOT EXISTS tracking_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS tracking_confirmed_by TEXT REFERENCES users(id);
