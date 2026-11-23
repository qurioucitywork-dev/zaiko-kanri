# Cloudflare / Neon performance verification — 2026-11-24

## Result

The deployed application completes initialization and allows navigation to the dashboard, inventory, master records and document lists. Existing record counts and dashboard totals matched the pre-change view. No business data or schema was changed for this release.

Deployment: `2033f129-d08b-40fc-a11c-0d251e0a8fa8`.

| Check | Before | After |
| --- | ---: | ---: |
| Authenticated browser reload to complete initialization | 22.9 s | 7.5 s |
| Same 12 authenticated API reads, maximum concurrency 8 | 3.91 s | 1.63 s |
| Dashboard API within that read batch | 2.89 s | 1.05 s |
| Inventory / masters / document-list navigation after initialization | Not measured | 0.83–0.95 s |

These are individual measurements against the same restored test dataset, not an SLA or a load test. Browser figures include automation/observation overhead. The app was running during measurement; an idle start or a suspended database can take longer. The early dashboard and full-ready observations were both available by the end of the browser reload call, so no separate first-paint claim is made.

## Changes

- Session authorization and expiry are still checked in PostgreSQL on every request. Updating activity time is limited to once per minute using a conditional update, avoiding a transaction and a shared row lock for every concurrent API read.
- Explicit user permission overrides and role permissions are resolved in one query. Revocations and explicit denies are not cached.
- Four inventory count queries were combined into one query with the same eligibility conditions. Results were compared against the original queries on Neon and with null/deleted/other-organization fixtures.
- Independent frontend requests overlap, with a maximum of eight reads. Queue wait does not consume the network timeout. All five document-detail groups load concurrently.
- The dashboard can render from complete product/transaction facts before image/document details finish. Initial mutations and other-page navigation remain blocked until the complete dataset is ready. Error recovery preserves the last complete snapshot; late image responses cannot overwrite a restored snapshot.
- Readiness is checked once per live container process and invalidated when it exits. Application responses and business data are not cached by this gate.
- The already-migrated Cloudflare environment skips startup migrations. A new application instance requests Southeast Asia placement, near the Singapore database, using a best-effort location hint. Existing Docker startup defaults, instance size and pricing plan remain unchanged.

## Validation

- All 27 frontend test files passed; the additional late-image rollback test passed after the final patch.
- Linux Docker `go test ./...` passed after the final embedded asset/hash update. Environment-dependent PostgreSQL integration tests outside the targeted checks still require their configured test environment; this does not replace the remaining full workflow testing noted in the deployment PR.
- Cloudflare TypeScript checks and 10 storage/readiness tests passed.
- The live API benchmark returned HTTP 200 for all requests, with unchanged result counts.
- Three existing worker accounts and two guest accounts passed login and identity checks. Workers retained inventory access; guests were denied the staff inventory endpoint and could read their guest catalog. Verification sessions were logged out afterward.
- The real authenticated browser displayed the dashboard, inventory, master records and purchase document list; no warning/error console messages were captured during this verification.
- No credentials, database backups, runtime logs or account data are included in this document or the commit.

See [runtime deployment notes](../deploy/cloudflare/README.md) for explicit migration and restart requirements. Private raw timing logs remain under the ignored `.runtime` directory.
