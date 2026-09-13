DROP TRIGGER IF EXISTS products_purchase_inventory_consistency ON products;
DROP TRIGGER IF EXISTS purchase_lines_inventory_consistency ON purchase_slip_lines;
DROP TRIGGER IF EXISTS purchase_slips_inventory_consistency ON purchase_slips;

DROP FUNCTION IF EXISTS enforce_product_purchase_inventory_consistency();
DROP FUNCTION IF EXISTS enforce_purchase_line_inventory_consistency();
DROP FUNCTION IF EXISTS enforce_purchase_slip_inventory_consistency();
DROP FUNCTION IF EXISTS assert_purchase_inventory_consistency(TEXT);

DROP INDEX IF EXISTS idx_products_one_per_purchase_line;
