CREATE UNIQUE INDEX IF NOT EXISTS ux_stocktakes_one_draft_per_org
    ON stocktakes (organization_id)
    WHERE status = 'draft';
