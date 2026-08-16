DROP INDEX IF EXISTS idx_business_partners_filters;
ALTER TABLE business_partners
    DROP COLUMN IF EXISTS is_other,
    DROP COLUMN IF EXISTS closing_day,
    DROP COLUMN IF EXISTS region_type;
