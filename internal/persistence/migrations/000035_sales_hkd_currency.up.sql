ALTER TABLE exchange_rate_snapshots DROP CONSTRAINT IF EXISTS exchange_rate_snapshots_base_currency_check;
ALTER TABLE exchange_rate_snapshots ADD CONSTRAINT exchange_rate_snapshots_base_currency_check CHECK (base_currency IN ('JPY','USD','EUR','HKD'));

ALTER TABLE sales_slips DROP CONSTRAINT IF EXISTS sales_slips_display_currency_check;
ALTER TABLE sales_slips ADD CONSTRAINT sales_slips_display_currency_check CHECK (display_currency IN ('JPY','USD','EUR','HKD'));

ALTER TABLE sales_lines DROP CONSTRAINT IF EXISTS sales_lines_sale_currency_check;
ALTER TABLE sales_lines ADD CONSTRAINT sales_lines_sale_currency_check CHECK (sale_currency IN ('JPY','USD','EUR','HKD'));

INSERT INTO exchange_rate_snapshots
  (id, organization_id, base_currency, quote_currency, rate_scaled, scale, provider, observed_at, created_by, created_at)
SELECT 'fxr_hkd_' || md5(o.id), o.id, 'HKD', 'JPY', 1980000000, 100000000,
       'initial HKD master rate', NOW(), u.id, NOW()
FROM organizations o
JOIN LATERAL (
  SELECT id FROM users
  WHERE organization_id=o.id
  ORDER BY created_at, id
  LIMIT 1
) u ON TRUE
WHERE NOT EXISTS (
  SELECT 1 FROM exchange_rate_snapshots r
  WHERE r.organization_id=o.id AND r.base_currency='HKD' AND r.quote_currency='JPY'
);
