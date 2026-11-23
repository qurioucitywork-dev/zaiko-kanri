ALTER TABLE market_price_records
    DROP CONSTRAINT IF EXISTS market_price_records_market_currency_check;

ALTER TABLE market_price_records
    ADD CONSTRAINT market_price_records_market_currency_check
    CHECK (market_currency IN ('JPY', 'USD', 'HKD'));

ALTER TABLE market_price_records
    ADD COLUMN IF NOT EXISTS market_fx_rate_scaled BIGINT NOT NULL DEFAULT 100000000,
    ADD COLUMN IF NOT EXISTS market_fx_scale BIGINT NOT NULL DEFAULT 100000000;

ALTER TABLE market_import_rows
    ADD COLUMN IF NOT EXISTS market_fx_rate_scaled BIGINT NOT NULL DEFAULT 100000000,
    ADD COLUMN IF NOT EXISTS market_fx_scale BIGINT NOT NULL DEFAULT 100000000;

WITH historical_rates AS (
    SELECT
        market.id AS market_id,
        snapshot.rate_scaled,
        snapshot.scale
    FROM market_price_records AS market
    JOIN LATERAL (
        SELECT rate.rate_scaled, rate.scale
        FROM exchange_rate_snapshots AS rate
        WHERE rate.organization_id = market.organization_id
          AND rate.base_currency = market.market_currency
          AND rate.quote_currency = 'JPY'
          AND rate.observed_at < market.import_date + INTERVAL '1 day'
        ORDER BY rate.observed_at DESC, rate.created_at DESC
        LIMIT 1
    ) AS snapshot ON market.market_currency IN ('USD', 'HKD')
)
UPDATE market_price_records AS market
SET
    market_fx_rate_scaled = historical_rates.rate_scaled,
    market_fx_scale = historical_rates.scale
FROM historical_rates
WHERE market.id = historical_rates.market_id;

WITH latest_rates AS (
    SELECT DISTINCT ON (market.organization_id, market.market_currency)
        market.organization_id,
        market.market_currency,
        snapshot.rate_scaled,
        snapshot.scale
    FROM market_price_records AS market
    JOIN exchange_rate_snapshots AS snapshot
      ON snapshot.organization_id = market.organization_id
     AND snapshot.base_currency = market.market_currency
     AND snapshot.quote_currency = 'JPY'
    WHERE market.market_currency IN ('USD', 'HKD')
    ORDER BY market.organization_id, market.market_currency, snapshot.observed_at DESC, snapshot.created_at DESC
)
UPDATE market_price_records AS market
SET
    market_fx_rate_scaled = latest_rates.rate_scaled,
    market_fx_scale = latest_rates.scale
FROM latest_rates
WHERE market.organization_id = latest_rates.organization_id
  AND market.market_currency = latest_rates.market_currency
  AND market.market_fx_rate_scaled = 100000000
  AND market.market_fx_scale = 100000000;

UPDATE market_price_records
SET market_fx_rate_scaled = 100000000,
    market_fx_scale = 100000000
WHERE market_currency = 'JPY';

ALTER TABLE market_price_records
    ADD CONSTRAINT market_price_records_market_fx_rate_positive_check
    CHECK (market_fx_rate_scaled > 0 AND market_fx_scale > 0);

ALTER TABLE market_import_rows
    ADD CONSTRAINT market_import_rows_market_fx_rate_positive_check
    CHECK (market_fx_rate_scaled > 0 AND market_fx_scale > 0);

COMMENT ON COLUMN market_price_records.market_fx_rate_scaled IS
    '市場調査時点で固定した、相場通貨1単位に対するJPY換算レートの分子。';
COMMENT ON COLUMN market_price_records.market_fx_scale IS
    '市場調査時点のJPY換算レートの分母。';
