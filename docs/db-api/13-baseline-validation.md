# D1 baseline validation

Validated on 2026-07-31 without opening the application database or any remote
Cloudflare resource.

## Artifacts

- Reviewed replay candidate: `docs/db-api/baseline-candidate-000027.sql`
- Generated flat baseline:
  `deploy/cloudflare/d1-service/migrations/0001_baseline.sql`
- Deterministic generator: `cmd/dbbaseline/main.go`
- Regression tests: `internal/diagnostics/baseline_candidate_test.go`

The flat baseline contains the final empty-database schema only. Preview seed
data and legacy data-fix statements are not included. The existing numbered
SQLite migrations remain unchanged.

## Results

| Check | Result |
|---|---|
| Replay candidate on isolated modernc SQLite | PASS |
| SQLite `integrity_check` | `ok` |
| SQLite foreign-key violations | `0` |
| Final business tables | `49` |
| Final explicit indexes | `47` |
| Final triggers | `2` |
| Flat baseline equals replay final schema | PASS |
| Replay candidate on Wrangler local D1 | `174` statements PASS |
| Flat baseline on fresh Wrangler local D1 | `98` statements PASS |
| D1 product table present | PASS |
| D1 product column count | `32` |
| D1 service TypeScript check | PASS |
| Container router TypeScript check | PASS |

Wrangler was invoked with `--local` and an isolated persistence directory under
`.tmp`. It was never invoked with `--remote` or `deploy`.

## Reproduction

```powershell
$env:GOCACHE = (Resolve-Path '.\.gocache')
.\.tooling\go\bin\go.exe test -count=1 ./internal/diagnostics

.\.tooling\go\bin\go.exe run .\cmd\dbbaseline `
  -input docs\db-api\baseline-candidate-000027.sql `
  -output deploy\cloudflare\d1-service\migrations\0001_baseline.sql
```

For Wrangler, use the test-only example configuration, `--local`, and a new
`--persist-to` directory. Production IDs must not be inserted in the example
file.

## Deployment status

This proves syntax and schema equivalence for an empty local D1 database. It
does not authorize a remote migration. Before a Cloudflare test deployment,
create a separate test D1 database, take an export/Time Travel bookmark, import
sanitized test data, run contract tests, and document restore evidence.

AWS remains the final production target. PostgreSQL/RDS migration validation is
independent from this D1 test baseline.
