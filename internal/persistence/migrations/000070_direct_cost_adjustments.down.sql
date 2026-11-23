DROP TABLE IF EXISTS direct_cost_adjustment_lines;

ALTER TABLE cost_adjustments
    DROP CONSTRAINT IF EXISTS cost_adjustments_adjustment_type_check;

ALTER TABLE cost_adjustments
    ADD CONSTRAINT cost_adjustments_adjustment_type_check CHECK (
        adjustment_type IN ('breakdown','disassemble','combine','swap')
    ),
    ADD CONSTRAINT cost_adjustments_organization_id_source_product_id_key UNIQUE (organization_id, source_product_id);

