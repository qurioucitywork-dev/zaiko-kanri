ALTER TABLE staff_profiles ADD COLUMN inventory_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE staff_profiles ADD COLUMN overseas_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE staff_profiles ADD COLUMN tag_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE staff_profiles ADD COLUMN domestic_detail TEXT NOT NULL DEFAULT '';
UPDATE staff_profiles SET inventory_detail=display_name WHERE is_purchase_staff;
