UPDATE market_price_records
SET
    market_price_minor = ROUND(
        market_price_minor::NUMERIC * market_fx_rate_scaled::NUMERIC / market_fx_scale::NUMERIC
    )::BIGINT,
    market_currency = 'JPY'
WHERE market_currency = 'HKD';

ALTER TABLE market_price_records
    DROP CONSTRAINT IF EXISTS market_price_records_market_fx_rate_positive_check,
    DROP COLUMN IF EXISTS market_fx_rate_scaled,
    DROP COLUMN IF EXISTS market_fx_scale;

ALTER TABLE market_import_rows
    DROP CONSTRAINT IF EXISTS market_import_rows_market_fx_rate_positive_check,
    DROP COLUMN IF EXISTS market_fx_rate_scaled,
    DROP COLUMN IF EXISTS market_fx_scale;

ALTER TABLE market_price_records
    DROP CONSTRAINT IF EXISTS market_price_records_market_currency_check;

ALTER TABLE market_price_records
    ADD CONSTRAINT market_price_records_market_currency_check
    CHECK (market_currency IN ('JPY', 'USD'));
