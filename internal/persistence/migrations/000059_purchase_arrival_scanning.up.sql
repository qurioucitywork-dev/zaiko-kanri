-- Inventory that existed before physical-arrival scanning was introduced has
-- already completed intake. Only products confirmed after this migration
-- start in purchasing state and require a scan.
UPDATE products
SET inventory_status='in_stock', updated_at=NOW()
WHERE inventory_status='purchasing' AND deleted_at IS NULL;
