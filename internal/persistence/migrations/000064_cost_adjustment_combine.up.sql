CREATE TABLE IF NOT EXISTS cost_adjustment_input_parts (
    cost_adjustment_id TEXT NOT NULL REFERENCES cost_adjustments(id) ON DELETE CASCADE,
    part_id TEXT NOT NULL REFERENCES parts(id),
    source_part_code TEXT NOT NULL,
    source_cost_jpy_minor BIGINT NOT NULL CHECK (source_cost_jpy_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (cost_adjustment_id, part_id),
    UNIQUE (part_id)
);

CREATE INDEX IF NOT EXISTS idx_cost_adjustment_input_parts_adjustment
    ON cost_adjustment_input_parts(cost_adjustment_id);
