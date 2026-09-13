-- A confirmed purchase slip and inventory are one accounting fact.  Every
-- purchase line represents exactly one individually managed product, and the
-- relationship must remain one-to-one even when either side is edited later.

UPDATE purchase_slip_lines AS line
SET generated_product_count = linked.product_count
FROM (
    SELECT purchase_slip_line_id, COUNT(*)::INTEGER AS product_count
    FROM products
    WHERE purchase_slip_line_id IS NOT NULL AND deleted_at IS NULL
    GROUP BY purchase_slip_line_id
) AS linked
WHERE line.id = linked.purchase_slip_line_id;

UPDATE purchase_slip_lines AS line
SET generated_product_count = 0
WHERE NOT EXISTS (
    SELECT 1
    FROM products AS product
    WHERE product.purchase_slip_line_id = line.id AND product.deleted_at IS NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_products_one_per_purchase_line
    ON products(purchase_slip_line_id)
    WHERE purchase_slip_line_id IS NOT NULL AND deleted_at IS NULL;

CREATE OR REPLACE FUNCTION assert_purchase_inventory_consistency(target_purchase_id TEXT)
RETURNS VOID AS $$
DECLARE
    slip_status TEXT;
    expected_count BIGINT;
    actual_count BIGINT;
    invalid_generated_count BIGINT;
BEGIN
    IF target_purchase_id IS NULL OR target_purchase_id = '' THEN
        RETURN;
    END IF;

    SELECT status INTO slip_status
    FROM purchase_slips
    WHERE id = target_purchase_id;

    IF slip_status IS DISTINCT FROM 'confirmed' THEN
        RETURN;
    END IF;

    SELECT
        COALESCE(SUM(line.quantity), 0),
        COUNT(product.id),
        COUNT(*) FILTER (WHERE line.generated_product_count <> line.quantity)
    INTO expected_count, actual_count, invalid_generated_count
    FROM purchase_slip_lines AS line
    LEFT JOIN products AS product
      ON product.purchase_slip_line_id = line.id
     AND product.deleted_at IS NULL
    WHERE line.purchase_slip_id = target_purchase_id;

    IF expected_count = 0
       OR expected_count <> actual_count
       OR invalid_generated_count <> 0 THEN
        RAISE EXCEPTION
            'confirmed purchase % must match inventory (expected %, actual %, invalid generated counts %)',
            target_purchase_id, expected_count, actual_count, invalid_generated_count
            USING ERRCODE = '23514';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enforce_purchase_slip_inventory_consistency()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM assert_purchase_inventory_consistency(NEW.id);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enforce_purchase_line_inventory_consistency()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM assert_purchase_inventory_consistency(COALESCE(NEW.purchase_slip_id, OLD.purchase_slip_id));
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enforce_product_purchase_inventory_consistency()
RETURNS TRIGGER AS $$
DECLARE
    target_line_id TEXT;
    target_purchase_id TEXT;
BEGIN
    target_line_id := COALESCE(NEW.purchase_slip_line_id, OLD.purchase_slip_line_id);
    IF target_line_id IS NULL OR target_line_id = '' THEN
        RETURN NULL;
    END IF;

    SELECT purchase_slip_id INTO target_purchase_id
    FROM purchase_slip_lines
    WHERE id = target_line_id;

    PERFORM assert_purchase_inventory_consistency(target_purchase_id);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS purchase_slips_inventory_consistency ON purchase_slips;
CREATE CONSTRAINT TRIGGER purchase_slips_inventory_consistency
AFTER INSERT OR UPDATE OF status ON purchase_slips
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_purchase_slip_inventory_consistency();

DROP TRIGGER IF EXISTS purchase_lines_inventory_consistency ON purchase_slip_lines;
CREATE CONSTRAINT TRIGGER purchase_lines_inventory_consistency
AFTER INSERT OR UPDATE OR DELETE ON purchase_slip_lines
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_purchase_line_inventory_consistency();

DROP TRIGGER IF EXISTS products_purchase_inventory_consistency ON products;
CREATE CONSTRAINT TRIGGER products_purchase_inventory_consistency
AFTER INSERT OR UPDATE OR DELETE ON products
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_product_purchase_inventory_consistency();

-- Existing confirmed data must satisfy the same invariant before this
-- migration is accepted.  A mismatch stops startup instead of silently
-- presenting different purchase and inventory totals.
DO $$
DECLARE
    purchase RECORD;
BEGIN
    FOR purchase IN SELECT id FROM purchase_slips WHERE status = 'confirmed' LOOP
        PERFORM assert_purchase_inventory_consistency(purchase.id);
    END LOOP;
END;
$$;
