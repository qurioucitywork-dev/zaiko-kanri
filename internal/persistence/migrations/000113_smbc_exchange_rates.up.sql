ALTER TABLE exchange_rate_snapshots ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE exchange_rate_snapshots ADD COLUMN published_date TEXT NOT NULL DEFAULT '';
ALTER TABLE exchange_rate_snapshots ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
ALTER TABLE exchange_rate_snapshots ADD COLUMN source_sha256 TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX exchange_rates_smbc_publication ON exchange_rate_snapshots(organization_id,base_currency,published_date) WHERE provider='SMBC TTM';
CREATE TABLE exchange_rate_sync_state (
 organization_id TEXT PRIMARY KEY REFERENCES organizations(id),
 published_date TEXT NOT NULL DEFAULT '',
 source_url TEXT NOT NULL DEFAULT '',
 last_attempt_at TIMESTAMPTZ,
 last_success_at TIMESTAMPTZ,
 last_error TEXT NOT NULL DEFAULT '',
 failure_notified_date TEXT NOT NULL DEFAULT ''
);
