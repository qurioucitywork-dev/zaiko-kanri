# Cloudflare / Neon performance verification — 2026-11-24

## Result

The deployed application initializes correctly and allows navigation to the dashboard, inventory, master records and document lists. Existing counts and dashboard totals matched the pre-change view. No business data or schema was changed for either performance release.

Current deployment: `0e10c370-b876-4154-94f2-daa34e84257e`.

| Check | Original | First improvement | Current improvement |
| --- | ---: | ---: | ---: |
| Authenticated browser reload to complete initialization | 22.9 s | 7.5 s | 3.4–3.9 s |
| Dashboard metrics visibly available | Not measured separately | Same observation as full initialization | 0.95–1.83 s |
| Same 12 authenticated API reads, maximum concurrency 8 | 3.91 s | 1.63 s | Not repeated |

The three current browser samples were dashboard/full-ready 1,832/3,782 ms, 1,195/3,381 ms and 946/3,898 ms. The first sample downloaded the updated assets; the following two revalidated their cached copies. Full-ready still means that all document details and image metadata have loaded, not only the dashboard facts. Navigation to inventory, masters and document lists was also verified without warning/error console entries.

These are measurements against the same restored test dataset, not an SLA or load test. Browser figures include automation/observation overhead. The application and database were already running. The separate first health request to the newly deployed container took about 6 seconds; subsequent health requests were about 93 ms. Idle startup or a suspended database can therefore add time before application initialization.

## Data equivalence

Using the same authenticated session, 104 logical JSON reads were fetched through both the original GET endpoints and the new read-only batch endpoint. All statuses were 200 and all canonical JSON bodies matched exactly, including masters, products, transaction lists, document details and image metadata.

The controlled comparison used 104 individual HTTP requests versus 13 groups of eight. Their total times were 2,043 ms and 2,022 ms respectively with the updated connection pool. This test proves equivalence and fewer network round trips; it does not isolate the batch API as the cause of the overall browser speedup. The browser result combines connection reuse, transport changes, static revalidation and corrected dashboard visibility.

## Current changes and impacts

- Added an opt-in, read-only batch transport. It invokes the existing GET handlers, limited to explicitly allowed JSON reads, with at most eight simultaneous handlers. The browser keeps its eight-logical-read limit and separates core dashboard data from secondary reads. Existing servers without the feature flag keep using individual GET requests.
- Session validity and permissions are read fresh for each outer batch request and shared only among its child reads. They are never cached across requests. Permission changes apply on the next request; an already accepted batch uses its starting authorization snapshot, bounded by a 30-second context timeout. Existing worker/guest and organization boundaries remain enforced for each item. Elevated admin-page reads and all mutations retain their original routes.
- Request bodies are limited to 64 KiB, batches to 32 items, and aggregate response bodies to 8 MiB. Oversized results fall back to the original individual GETs. Permission failures and malformed successful responses do not silently fall back. Cancellation, error isolation and initialization rollback are covered by tests.
- PostgreSQL retains up to eight idle connections instead of five, matching one wave of reads. Maximum open connections remains twenty and connection lifetime remains thirty minutes. This can retain up to three additional idle connections per application process; no database plan or container size was changed. A separate read-only Neon probe showed five idle slots closing three connections each wave and taking about 682–683 ms; eight retained slots avoided these closures and took about 84–170 ms once warm. These probe times are not a load-test guarantee.
- Embedded HTML, JavaScript and CSS now send weak content ETags while retaining `Cache-Control: no-cache`. The browser checks freshness on every load; unchanged files can return 304 with no body. Dynamic API responses remain `no-store`, and no business-data cache was introduced. Live checks confirmed this for HTML, JavaScript and CSS.
- Corrected CSS that previously kept the dashboard hidden even after complete core facts had arrived. Initial dashboard metrics are now visible early. Other pages and mutations remain blocked until the complete dataset is ready; subsequent full refreshes retain their existing visibility gate.
- Deployment needed a new container instance name because the existing instance continued serving the old image. The replacement uses the same restored Neon DB, R2 bucket and secrets. The old instance receives no new scheduled traffic and expires after its existing inactivity timeout. See the deployment runbook for verification and rollout details.

## Earlier improvements retained

- Session authorization and expiry are still checked in PostgreSQL on every individual request. Updating activity time is limited to once per minute using a conditional update, avoiding a transaction and shared row lock for every concurrent API read.
- Explicit user permission overrides and role permissions are resolved in one query. Revocations and explicit denies are not cached across requests.
- Four inventory count queries were combined into one with the same eligibility conditions, verified against original queries on Neon and null/deleted/other-organization fixtures.
- Independent frontend requests and all five document-detail groups overlap. Queue wait does not consume network timeout. Error recovery preserves the last complete snapshot, and late image responses cannot overwrite it.
- Readiness is checked once per live container process and invalidated when it exits. Application responses and business data are not cached by this gate.
- The already-migrated Cloudflare environment skips startup migrations. A new instance requests Southeast Asia placement near the Singapore database using a best-effort hint. Existing Docker startup defaults are retained.

## Validation

- All 28 frontend test files passed, including both transport paths, dashboard computed visibility, full-data readiness, cancellation and rollback.
- Linux Docker `go test ./...` passed after the final embedded asset/hash update. Environment-dependent PostgreSQL integration tests outside the targeted checks still require their configured environment; this does not replace the remaining full workflow testing noted in the deployment PR.
- Cloudflare TypeScript checks and ten storage/readiness tests passed. The Linux deployment binary built successfully.
- Backend batch tests cover CSRF, authentication, next-request permission revocation, role/override decisions, cross-organization isolation, guest boundaries, strict path/query allowlists, timeout propagation, handler panic isolation and response size limits.
- Live direct/batch JSON parity and real-browser verification are described above. Three existing workers retained product/purchase reads (200) and were denied user/settings reads (403). Two existing guests were denied all four internal reads (403) and retained their guest catalog (200). Direct and batch statuses matched; all five verification sessions were logged out (204). Private evidence contains roles and statuses only.
- No credentials, database backups, runtime logs, account identities or business record values are included in this document or commit.

See [runtime deployment notes](../deploy/cloudflare/README.md) for explicit migration and restart requirements. Private raw measurements remain under the ignored `.runtime` directory.
