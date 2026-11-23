# Cloud runtime deployment notes

The Worker forwards every application request to the Go application. The readiness
cache only skips repeated container startup probes while the process stays alive;
it does not cache application responses, database data, permissions, or sessions.
Process exit or an observed stopped container invalidates the readiness cache.

`SKIP_DATABASE_MIGRATIONS=true` is set only in the Cloudflare configuration. Apply
new PostgreSQL migrations with `go run ./cmd/migrate` using the protected target
environment **before deploying a release that needs a new schema**. The command
uses the direct Neon migration connection. The existing Docker startup defaults
are unchanged. Do not turn this flag on for a database that has not been migrated.

Build the frontend and Linux binary with `./deploy/cloudflare/build.ps1`, which
updates `APP_REVISION`. Then deploy the complete Worker and application image.
If the database connection or its credentials change, change `APP_INSTANCE_NAME`
to a new deployment-specific name and verify the organization and business record
counts after login. A health response alone does not prove the intended database
is being used. Preserve the previous connection settings and database for rollback.

The 15-minute scheduled request keeps the process available for the existing Go
SMBC synchronization loop. No additional public maintenance or diagnostic route
is exposed. Database and R2 contents are never reset by readiness checks.

`APP_LOCATION_HINT=apac-se` asks Cloudflare to create the new application Durable
Object in Southeast Asia, close to the Neon database in Singapore. Both browser
requests and Cron use the same hint. This is best effort, not a guarantee of a
Singapore data center or a data residency boundary. Hints are honored only when a
new Durable Object is created, so the performance rollout uses a new
`APP_INSTANCE_NAME`; existing instances are not relocated. No instance size or
paid plan change is involved.

References:
- https://developers.cloudflare.com/containers/concepts/placement/
- https://developers.cloudflare.com/durable-objects/reference/data-location/
