DROP INDEX IF EXISTS idx_business_partners_contact_search;

ALTER TABLE business_partners
    DROP COLUMN IF EXISTS antique_license_number,
    DROP COLUMN IF EXISTS contact_phone;
