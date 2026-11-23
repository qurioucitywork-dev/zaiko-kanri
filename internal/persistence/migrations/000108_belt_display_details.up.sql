ALTER TABLE belt_materials ADD COLUMN inventory_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE belt_materials ADD COLUMN overseas_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE belt_materials ADD COLUMN tag_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE belt_materials ADD COLUMN domestic_detail TEXT NOT NULL DEFAULT '';
UPDATE belt_materials SET inventory_detail=name,overseas_detail=name,tag_detail=name,domestic_detail=name;
