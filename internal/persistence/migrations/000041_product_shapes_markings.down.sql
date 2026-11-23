DROP INDEX IF EXISTS idx_products_marking_id;
DROP INDEX IF EXISTS idx_products_shape_id;
ALTER TABLE purchase_slip_lines DROP COLUMN IF EXISTS marking_id;
ALTER TABLE purchase_slip_lines DROP COLUMN IF EXISTS shape_id;
ALTER TABLE products DROP COLUMN IF EXISTS marking_id;
ALTER TABLE products DROP COLUMN IF EXISTS shape_id;
DROP TABLE IF EXISTS markings;
DROP TABLE IF EXISTS product_shapes;
