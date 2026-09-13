CREATE TABLE IF NOT EXISTS cost_adjustments (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    source_product_id TEXT NOT NULL REFERENCES products(id),
    source_purchase_slip_id TEXT NOT NULL REFERENCES purchase_slips(id),
    source_purchase_slip_line_id TEXT NOT NULL REFERENCES purchase_slip_lines(id),
    adjustment_type TEXT NOT NULL DEFAULT 'breakdown'
        CHECK (adjustment_type IN ('breakdown', 'disassemble', 'combine', 'swap')),
    adjustment_date DATE NOT NULL,
    source_product_code TEXT NOT NULL,
    source_cost_jpy_minor BIGINT NOT NULL CHECK (source_cost_jpy_minor >= 0),
    allocated_cost_jpy_minor BIGINT NOT NULL CHECK (allocated_cost_jpy_minor >= 0),
    status TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('draft', 'confirmed', 'cancelled')),
    confirmed_at TIMESTAMPTZ,
    confirmed_by TEXT REFERENCES users(id),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, source_product_id)
);

CREATE TABLE IF NOT EXISTS cost_adjustment_items (
    id TEXT PRIMARY KEY,
    cost_adjustment_id TEXT NOT NULL REFERENCES cost_adjustments(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position BETWEEN 1 AND 20),
    item_kind TEXT NOT NULL CHECK (item_kind IN ('product', 'part')),
    output_product_id TEXT REFERENCES products(id),
    output_part_id TEXT,
    management_code TEXT NOT NULL,
    allocated_cost_jpy_minor BIGINT NOT NULL CHECK (allocated_cost_jpy_minor >= 0),
    status TEXT NOT NULL DEFAULT 'cost_adjustment' CHECK (status IN ('cost_adjustment', 'completed', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (cost_adjustment_id, position),
    CHECK (
        (item_kind='product' AND output_product_id IS NOT NULL AND output_part_id IS NULL)
        OR (item_kind='part' AND output_product_id IS NULL AND output_part_id IS NOT NULL)
    )
);

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS internal_comment TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cost_adjustment_id TEXT REFERENCES cost_adjustments(id);

ALTER TABLE parts
    ADD COLUMN IF NOT EXISTS internal_comment TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cost_adjustment_id TEXT REFERENCES cost_adjustments(id),
    ADD COLUMN IF NOT EXISTS purchase_slip_line_id TEXT REFERENCES purchase_slip_lines(id);

ALTER TABLE cost_adjustment_items
    ADD CONSTRAINT cost_adjustment_items_output_part_fk
    FOREIGN KEY (output_part_id) REFERENCES parts(id);

ALTER TABLE purchase_slip_lines
    ADD COLUMN IF NOT EXISTS line_item_kind TEXT NOT NULL DEFAULT 'product'
        CHECK (line_item_kind IN ('product', 'part')),
    ADD COLUMN IF NOT EXISTS generated_part_count INTEGER NOT NULL DEFAULT 0
        CHECK (generated_part_count >= 0),
    ADD COLUMN IF NOT EXISTS cost_adjustment_id TEXT REFERENCES cost_adjustments(id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_parts_one_per_purchase_line
    ON parts(purchase_slip_line_id)
    WHERE purchase_slip_line_id IS NOT NULL;

ALTER TABLE products DROP CONSTRAINT IF EXISTS products_inventory_status_check;
ALTER TABLE products ADD CONSTRAINT products_inventory_status_check
    CHECK (inventory_status IN (
        'purchasing', 'in_stock', 'cost_adjustment', 'broken_down', 'reserved', 'return_pending',
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
    IF target_purchase_id IS NULL OR target_purchase_id = '' THEN
        RETURN;
    END IF;

    SELECT status INTO slip_status FROM purchase_slips WHERE id = target_purchase_id;
    IF slip_status IS DISTINCT FROM 'confirmed' THEN
        RETURN;
    END IF;

    SELECT
        COALESCE(SUM(line.quantity), 0),
        COUNT(DISTINCT product.id) + COUNT(DISTINCT part.id),
        COUNT(*) FILTER (WHERE
            (line.line_item_kind='product' AND
                (line.generated_product_count <> line.quantity OR line.generated_part_count <> 0))
            OR
            (line.line_item_kind='part' AND
                (line.generated_part_count <> line.quantity OR line.generated_product_count <> 0))
        )
    INTO expected_count, actual_count, invalid_generated_count
    FROM purchase_slip_lines AS line
    LEFT JOIN products AS product
      ON product.purchase_slip_line_id = line.id AND product.deleted_at IS NULL
    LEFT JOIN parts AS part
      ON part.purchase_slip_line_id = line.id
    WHERE line.purchase_slip_id = target_purchase_id;

    IF expected_count = 0 OR expected_count <> actual_count OR invalid_generated_count <> 0 THEN
        RAISE EXCEPTION
            'confirmed purchase % must match inventory and parts (expected %, actual %, invalid generated counts %)',
            target_purchase_id, expected_count, actual_count, invalid_generated_count
            USING ERRCODE = '23514';
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enforce_product_purchase_date_consistency()
RETURNS TRIGGER AS $$
DECLARE
    source_purchase_date DATE;
BEGIN
    IF NEW.purchase_slip_line_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT slip.purchase_date INTO source_purchase_date
    FROM purchase_slip_lines AS line
    JOIN purchase_slips AS slip ON slip.id = line.purchase_slip_id
    WHERE line.id = NEW.purchase_slip_line_id;

    IF source_purchase_date IS NULL THEN
        RAISE EXCEPTION 'purchase slip date was not found for product %', NEW.id
            USING ERRCODE = '23503';
    END IF;

    IF NEW.cost_adjustment_id IS NULL AND NEW.purchase_date IS DISTINCT FROM source_purchase_date THEN
        RAISE EXCEPTION 'product purchase date must match its purchase slip date'
            USING ERRCODE = '23514';
    END IF;

    IF LEFT(NEW.product_code, 6) <> TO_CHAR(NEW.purchase_date, 'DDMMYY') THEN
        RAISE EXCEPTION 'product code date prefix must match its business date'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

