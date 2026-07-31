ALTER TABLE stocktakes ADD COLUMN expected_total_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE stocktakes ADD COLUMN saved_at TEXT;

ALTER TABLE stocktake_lines ADD COLUMN difference_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE stocktake_lines ADD COLUMN review_status TEXT NOT NULL DEFAULT 'none'
    CHECK (review_status IN ('none', 'pending', 'approved'));
ALTER TABLE stocktake_lines ADD COLUMN finalized_at TEXT;

UPDATE stocktakes
SET expected_total_minor = COALESCE((
    SELECT SUM(p.cost_amount_minor)
    FROM stocktake_lines sl
    JOIN products p ON p.id = sl.product_id
    WHERE sl.stocktake_id = stocktakes.id
), 0);

CREATE INDEX IF NOT EXISTS idx_stocktake_lines_review
    ON stocktake_lines (stocktake_id, review_status, counted_present);
