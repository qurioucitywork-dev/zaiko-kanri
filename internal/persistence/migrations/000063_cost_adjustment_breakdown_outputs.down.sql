UPDATE products SET inventory_status='cost_adjustment' WHERE inventory_status='broken_down';

DROP INDEX IF EXISTS idx_parts_one_per_purchase_line;

ALTER TABLE purchase_slip_lines
    DROP COLUMN IF EXISTS cost_adjustment_id,
    DROP COLUMN IF EXISTS generated_part_count,
    DROP COLUMN IF EXISTS line_item_kind;

ALTER TABLE cost_adjustment_items DROP CONSTRAINT IF EXISTS cost_adjustment_items_output_part_fk;

ALTER TABLE parts
    DROP COLUMN IF EXISTS purchase_slip_line_id,
    DROP COLUMN IF EXISTS cost_adjustment_id,
    DROP COLUMN IF EXISTS internal_comment;

ALTER TABLE products
    DROP COLUMN IF EXISTS cost_adjustment_id,
    DROP COLUMN IF EXISTS internal_comment;

DROP TABLE IF EXISTS cost_adjustment_items;
DROP TABLE IF EXISTS cost_adjustments;

ALTER TABLE products DROP CONSTRAINT IF EXISTS products_inventory_status_check;
ALTER TABLE products ADD CONSTRAINT products_inventory_status_check
    CHECK (inventory_status IN (
        'purchasing', 'in_stock', 'cost_adjustment', 'reserved', 'return_pending',
        'consigned', 'sold', 'shipped', 'cancelled', 'invalid'
    ));

CREATE OR REPLACE FUNCTION assert_purchase_inventory_consistency(target_purchase_id TEXT)
RETURNS VOID AS $$
DECLARE
    slip_status TEXT;
    expected_count BIGINT;
    actual_count BIGINT;
    invalid_generated_count BIGINT;
BEGIN
    IF target_purchase_id IS NULL OR target_purchase_id = '' THEN RETURN; END IF;
    SELECT status INTO slip_status FROM purchase_slips WHERE id = target_purchase_id;
    IF slip_status IS DISTINCT FROM 'confirmed' THEN RETURN; END IF;
    SELECT COALESCE(SUM(line.quantity),0),COUNT(product.id),
        COUNT(*) FILTER (WHERE line.generated_product_count <> line.quantity)
    INTO expected_count,actual_count,invalid_generated_count
    FROM purchase_slip_lines AS line
    LEFT JOIN products AS product ON product.purchase_slip_line_id=line.id AND product.deleted_at IS NULL
    WHERE line.purchase_slip_id=target_purchase_id;
    IF expected_count=0 OR expected_count<>actual_count OR invalid_generated_count<>0 THEN
        RAISE EXCEPTION 'confirmed purchase % must match inventory',target_purchase_id USING ERRCODE='23514';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enforce_product_purchase_date_consistency()
RETURNS TRIGGER AS $$
DECLARE source_purchase_date DATE;
BEGIN
    IF NEW.purchase_slip_line_id IS NULL THEN RETURN NEW; END IF;
    SELECT slip.purchase_date INTO source_purchase_date
    FROM purchase_slip_lines AS line JOIN purchase_slips AS slip ON slip.id=line.purchase_slip_id
    WHERE line.id=NEW.purchase_slip_line_id;
    IF source_purchase_date IS NULL THEN
        RAISE EXCEPTION 'purchase slip date was not found for product %',NEW.id USING ERRCODE='23503';
    END IF;
    IF NEW.purchase_date IS DISTINCT FROM source_purchase_date THEN
        RAISE EXCEPTION 'product purchase date must match its purchase slip date' USING ERRCODE='23514';
    END IF;
    IF LEFT(NEW.product_code,6)<>TO_CHAR(source_purchase_date,'DDMMYY') THEN
        RAISE EXCEPTION 'product code date prefix must match its purchase slip date' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
