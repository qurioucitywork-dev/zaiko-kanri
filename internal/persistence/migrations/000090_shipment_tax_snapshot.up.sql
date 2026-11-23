-- Keep previously registered shipment amounts unchanged. New slips snapshot the buyer's region.
ALTER TABLE shipment_slips ADD COLUMN IF NOT EXISTS tax_rate_basis_points integer NOT NULL DEFAULT 0 CHECK (tax_rate_basis_points IN (0, 1000));
