ALTER TABLE accessories ADD COLUMN inventory_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE accessories ADD COLUMN overseas_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE accessories ADD COLUMN tag_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE accessories ADD COLUMN domestic_detail TEXT NOT NULL DEFAULT '';
UPDATE accessories SET inventory_detail=name,overseas_detail=name,tag_detail=name,domestic_detail=name;
-- Removed catalog entries stay resolvable only as existing product references.
ALTER TABLE accessories ADD COLUMN is_legacy BOOLEAN NOT NULL DEFAULT FALSE;
