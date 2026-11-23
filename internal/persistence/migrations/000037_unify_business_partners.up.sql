ALTER TABLE business_partners
    ADD COLUMN IF NOT EXISTS region_type TEXT NOT NULL DEFAULT 'domestic',
    ADD COLUMN IF NOT EXISTS closing_day SMALLINT,
    ADD COLUMN IF NOT EXISTS is_other BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE business_partners
    DROP CONSTRAINT IF EXISTS business_partners_region_type_check;

ALTER TABLE business_partners
    ADD CONSTRAINT business_partners_region_type_check
    CHECK (region_type IN ('domestic', 'overseas'));

ALTER TABLE business_partners
    DROP CONSTRAINT IF EXISTS business_partners_closing_day_check;

ALTER TABLE business_partners
    ADD CONSTRAINT business_partners_closing_day_check
    CHECK (closing_day IS NULL OR closing_day BETWEEN 1 AND 31);

CREATE INDEX IF NOT EXISTS idx_business_partners_filters
    ON business_partners (organization_id, region_type, is_other, legal_name);
