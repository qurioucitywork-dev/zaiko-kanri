# PostgreSQL dataaccess adapter

## 1. Status and scope

`internal/dataaccess/postgresadapter` is the provider-neutral
`database/sql` adapter for the candidate PostgreSQL schema under
`zaiko.*`. It implements:

- `dataaccess.DiagnosticReader`
- `dataaccess.ProductReader`
- `dataaccess.ProductWriter`
- `dataaccess.InventoryWorkflowWriter`
- `dataaccess.ObjectMetadataReader`
- `dataaccess.ObjectMetadataWriter`

This stage does **not** wire a concrete PostgreSQL driver, open an RDS
connection, run migrations, change the existing SQLite store, or switch any
handler to PostgreSQL. The composition root must choose and register a driver
in a later stage.

`deploy/aws/postgres/verify_schema.sql` is a read-only, fail-fast gate. It
raises an error when a required mutation permission is absent, a tenant-owned
table lacks `organization_id`, or a minor-unit monetary column is not
`BIGINT`; a successful `psql` exit therefore requires all three invariants.

## 2. Construction boundary

```go
adapter, err := postgresadapter.New(db, postgresadapter.Config{
    StorageProvider: "s3",
    StorageBucket:   objectBucket,
    ObjectKeyPrefix: "objects",
})
```

The caller owns `*sql.DB`, including:

- concrete driver registration;
- DSN and secrets;
- TLS and IAM authentication;
- pool limits and connection lifetime;
- health monitoring and shutdown.

`New` does no network I/O. Bucket and prefix are stored only so a pending
`zaiko.product_objects` row uses the same private locator as
`internal/dataaccess/s3blob`. The S3 adapter and PostgreSQL adapter must receive
the same bucket and prefix. Credentials, endpoint, region, signed URLs, and
provider request IDs never cross the metadata port.

## 3. Tenant isolation

Every business query starts with `organization_id`. Composite joins also match
`organization_id`, rather than joining on an ID alone.

- `GetProduct` and `GetObjectMetadata` return `ErrNotFound` for both missing
  and cross-tenant IDs.
- Every mutation verifies that the organization and actor are active and that
  the actor is not logically deleted.
- Existing permission catalog keys are reused: product/image/return inventory
  mutations require `inventory.write`; purchase, sales, and shipment
  confirmation require `purchase.confirm`, `sales.confirm`, and
  `shipment.confirm`.
- An explicit user permission overrides the role. A user-level `allow`
  authorizes the operation, a user-level `deny` blocks it, and only when no
  user override exists is the matching role permission consulted.
- Product/object ownership checks use `(organization_id, id)`.
- Idempotency rows use
  `(organization_id, operation_name, idempotency_key)`.
- Audit rows always include the command tenant and actor.

Diagnostics are the only intentional exception: `SELECT 1` and the global
`zaiko.schema_migrations` probe contain no tenant data.

RLS is still a later defense-in-depth step. Until RLS is introduced, explicit
query predicates and the schema’s composite foreign keys are the isolation
boundary.

## 4. Product reads

The product reader uses the PostgreSQL physical model:

- `zaiko.products`
- `zaiko.suppliers`
- optional purchase-slip lineage through
  `zaiko.purchase_slip_lines`, `zaiko.purchase_slips`, and `zaiko.users`
- ready image count from `zaiko.product_objects`

Search properties:

- all criteria are parameterized with PostgreSQL `$n` placeholders;
- free text uses escaped `ILIKE` over code, SKU, brand, model, and serial;
- date input must be ISO `YYYY-MM-DD`;
- reversed or invalid date ranges are rejected before reaching the database;
- order is always `purchase_date DESC, product_code ASC`;
- page and offset arithmetic is overflow-checked;
- count and result queries share the same tenant/filter predicate.

`BIGINT` monetary columns scan directly into `int64`. They are never converted
through floating point. `DATE` scans to the contract’s ISO date string, while
`TIMESTAMPTZ` values are normalized to UTC.

Offset pagination matches the current `ProductReader` contract. If high-volume
keyset pagination is later required, it needs a new provider-neutral cursor
contract; it must not silently change this adapter’s page semantics.

## 5. Product registration

`CreateProduct` uses one `SERIALIZABLE` transaction for the whole business
operation:

1. verify the tenant actor and reserve the canonical idempotency key;
2. validate the optional buyer, supplier, accessory, and box references with
   tenant-qualified queries;
3. allocate or reserve the product code;
4. insert `zaiko.products`;
5. insert normalized `zaiko.product_accessories` and
   `zaiko.guest_box_products` relations;
6. append the initial `zaiko.inventory_events` row and an audit row;
7. persist the product ID, product code, and version in the idempotency row;
8. check cancellation immediately before commit and commit.

Automatic product codes use `YYYYMMDD` plus a three-digit sequence. The
sequence is advanced by one PostgreSQL upsert against
`zaiko.business_number_sequences`; row locking performed by the upsert makes
concurrent allocation safe. Sequence exhaustion at 999 returns `ErrConflict`.
An explicitly requested code must match the purchase date and also advances
the stored high-water mark with `GREATEST`, in the same transaction.

All monetary values are bound as `int64` to the schema's `BIGINT` columns.
Product uniqueness, serialization failure, and deadlock errors map to
`ErrConflict`. A missing or cross-tenant actor, buyer, supplier, accessory, or
box maps to `ErrNotFound`. A committed retry returns the stored product
identity without repeating any business inserts.

### 5.1 Authoritative-schema limitations

The current migration has no canonical `products.buyer_id` or
`products.created_by` column and no equivalent direct-registration relation.
`CreateProduct` therefore validates `ProductDraft.BuyerID`, but cannot persist
that association. The initial inventory event correctly records the command
actor, and `ProductReader` cannot round-trip `BuyerID` for a directly
registered product. If direct product
ownership by a buyer is required, a separately reviewed migration and read
contract decision are needed; this adapter does not invent a column.

The migration also retains legacy text columns `products.accessories` and
`products.box_text`. New writes leave those text columns empty and persist the
canonical normalized relations instead. The existing reader still exposes the
legacy text values. A future schema/read-model decision must define whether
reads should join normalized relations or retire the legacy fields; this stage
does not change DDL or silently synthesize text.

### 5.2 Inventory workflow writes

`ConfirmPurchase`, `ConfirmSale`, `ConfirmShipment`, and
`RestoreReturnedInventory` each use one `SERIALIZABLE` transaction. Within
that transaction the adapter verifies the tenant actor, reserves a canonical
idempotency record, locks prerequisite rows, applies status/version
compare-and-swap predicates, writes the header and detail records supported by
the command, appends inventory events and one audit record, commits the stored
result, and then commits the transaction.

Business numbers use a single yearly sequence-row upsert:

| Operation | Sequence kind | Format |
| --- | --- | --- |
| purchase confirmation | `purchase_slip` | `PI-YYYY-NNNN` |
| sale confirmation | `sales_slip` | `SL-YYYY-NNNN` |
| shipment confirmation | `shipment_slip` | `SH-YYYY-NNNN` |

The sequence is limited to 9999 and exhaustion returns `ErrConflict`.
Monetary values remain `int64` throughout. Business dates are parsed from ISO
strings and bound to PostgreSQL `DATE`; command and audit timestamps are bound
in UTC.

The implemented, unambiguous contract surface is deliberately narrower than
the DTOs:

- purchase confirmation accepts one physical `purchasing` product per line,
  with quantity 1, and currently requires `StaffID == Scope.ActorID`;
- sale confirmation accepts tax-exempt JPY, one physical
  `in_stock`/`reserved` product per line, and `ExpectedVersion == 0`;
- shipment confirmation checks `ExpectedVersion` against the identified
  source sales slip, requires that slip to be `confirmed`, and ships complete
  one-product/quantity-1 sales lines only;
- inventory restoration accepts one return item with quantity 1, checks the
  item `ExpectedVersion`, allows an invoice-completed item that has not yet
  been inventory-restored, and transitions its product from
  `sold`/`shipped`/`reserved` to `in_stock`.

Unsupported but valid DTO combinations return `ErrPrecondition`; they are not
silently interpreted. Missing or cross-tenant IDs return `ErrNotFound`.
Status/version/allocation/unique/serialization races return `ErrConflict`.
Committed retries return the stored ID, number and version without repeating
business writes.

#### 5.2.1 DDL and contract decisions still required

No columns or business rules were invented for the following gaps:

- `ConfirmPurchaseCommand` says products/numbers are allocated, but each line
  contains only an existing `ProductID`, amount and quantity. Required product
  attributes and N generated product IDs are absent. The DDL also links a
  product to a purchase line only through
  `products.purchase_slip_line_id`; `purchase_slip_lines` has no `product_id`.
  Therefore quantity greater than 1 and product generation are deferred.
- `StaffID` has no dedicated purchase-slip column. `created_by` and
  `confirmed_by` identify actors but do not define a separate staff assignment.
  Until that mapping is specified, the adapter only accepts the command actor
  as staff.
- `purchase_slip_lines.base_sale_price_minor` has no corresponding currency
  column, so it cannot independently preserve the product base-sale currency.
- `ConfirmSaleCommand.ExpectedVersion` has no existing `SalesSlipID` target
  and one scalar cannot safely describe multiple product versions. Non-zero
  values are deferred rather than reinterpreted.
- Standard tax rate and rounding are not defined. Foreign-currency conversion
  also lacks a rounding rule and `exchange_rate_snapshot_id`. Standard-tax and
  non-JPY sale confirmation are deferred.
- `ConfirmShipmentCommand` has product IDs but no quantities. Partial shipment
  and quantity-greater-than-1 allocation semantics are deferred.
- `ConditionCode` has no normalized condition-code column or foreign key and is
  currently stored in the legacy `products.condition_text` field.
- A restore command may contain multiple independently versioned return rows,
  while `WorkflowMutationResult` exposes one version and the DDL has no return
  batch/header. Multi-item restoration is deferred.
- Return rows can have quantity greater than 1 while one product row models a
  single physical inventory item. Multi-quantity restoration is deferred.

These items require a separately reviewed provider-neutral contract decision
before any DDL migration or broader adapter behavior.

## 6. Object metadata reads

Object metadata reads expose no bucket, key, storage version, or provider
locator. Results include lifecycle state and are ordered by:

1. `sort_order ASC`
2. `id ASC`

Nullable PostgreSQL checksum, size, ready time, and deleted time are converted
to the existing provider-neutral zero values. Unknown lifecycle values fail
closed instead of being passed to callers.

## 7. Object metadata writes

Each writer call uses one `SERIALIZABLE` transaction:

1. validate the command and UTC timestamps;
2. verify that the actor belongs to the tenant;
3. reserve the tenant/operation idempotency key;
4. apply one state transition with a status predicate;
5. append an audit row;
6. mark the idempotency row committed;
7. check context cancellation immediately before commit;
8. commit.

The current physical table has no numeric object version. Therefore object CAS
uses the lifecycle status:

| Operation | Allowed before state | Result |
| --- | --- | --- |
| create pending | no row | `pending` |
| mark ready | `pending` | `ready` |
| mark failed | `pending` | `failed` |
| mark deleted | `pending`, `ready`, `failed` | `deleted` |

A row that exists in the same tenant but is no longer in an allowed state
returns `ErrConflict`. A missing or cross-tenant row returns `ErrNotFound`.
PostgreSQL unique violation (`23505`), serialization failure (`40001`), and
deadlock (`40P01`) are also mapped through a dependency-free `SQLState()`
interface to `ErrConflict`.

### 7.1 Idempotency

The canonical request hash binds:

- operation name;
- tenant ID;
- actor ID;
- normalized command payload.

`RequestedAt` is deliberately excluded so a transport retry with a new retry
timestamp can replay the original result. The original timestamp is still
stored in `idempotency_records.requested_at`.

For all workflow writes (`purchase.confirm`, `sale.confirm`,
`shipment.confirm`, and `return.restore_inventory`), the command's embedded
`CommandScope` is removed before hashing. Tenant and actor remain bound by the
outer canonical envelope, and every workflow-specific business field remains
in the payload. Thus only the retry-attempt timestamp is variable: changing
tenant, actor, or any business value produces a different hash.

Reservation uses `INSERT ... ON CONFLICT DO NOTHING`, followed by
`SELECT ... FOR UPDATE` when the key already exists:

- same key and same hash in `committed` state: return the stored object;
- same key and different hash: `ErrIdempotencyMismatch`;
- non-committed duplicate key: `ErrConflict`.

The reservation is in the same transaction as the object and audit changes.
Rollback therefore cannot leave a false committed result.

### 7.2 S3 two-phase lifecycle

The adapter implements only metadata transitions:

1. `CreatePendingObject`;
2. S3 `Put`;
3. S3 `Head`;
4. `MarkObjectReady`.

Upload failure is recorded with `MarkObjectFailed`. Logical deletion uses
`MarkObjectDeleted`; deletion of S3 bytes remains the blob adapter’s
responsibility. Reconciliation remains a separate operational job.

`objectservice` derives the opaque object ID from a domain-separated SHA-256
of the normalized tenant, actor, and whole-operation idempotency key. The same
request therefore supplies the same `ObjectID` to `CreatePendingObject` across
processes and retries; a generated random ID is not injected into the
idempotency payload.

When a whole-operation retry reads already-`ready` metadata, the service
verifies the existing blob checksum and size with `Head` and returns the
committed metadata. It does not upload the retry body or repeat
`MarkObjectReady`.

An error returned by `MarkObjectReady` is treated as an ambiguous commit
result. The service does **not** delete the blob or force metadata to `failed`,
because the database commit may have succeeded before the response was lost.
The same-key retry above or reconciliation resolves that state.

For determinate upload and verification failures, blob deletion and
`MarkObjectFailed` are still attempted. Each compensation call uses its own
five-second context derived with `context.WithoutCancel`, so request
cancellation or deadline expiry does not suppress cleanup and a slow deletion
does not prevent the metadata attempt.

Allowed content types are `image/jpeg`, `image/png`, and `image/webp`.
Ready receipts require a lowercase SHA-256 and a size from 1 byte through
15 MiB, matching the physical schema and object-store contract.

## 8. Diagnostics

`Diagnose` performs only two reads:

- connectivity: `SELECT 1`;
- schema availability: read one row from `zaiko.schema_migrations`.

It never creates a probe row, runs a migration, changes `search_path`, or
reports a DSN/credential/provider locator. Component failures are returned in
the provider-neutral diagnostic report. Context cancellation is returned
directly.

## 9. Runtime requirements for the later composition stage

Before enabling this adapter:

1. apply and verify the candidate PostgreSQL migration through the approved
   one-shot migration job;
2. use PostgreSQL 16 or later;
3. force UTC at the database/session configuration;
4. configure TLS verification and secret/IAM credential retrieval;
5. give the runtime role only required DML/sequence privileges on `zaiko`;
6. keep migration DDL permissions on a separate role;
7. set finite pool, connection lifetime, statement timeout, lock timeout, and
   idle-in-transaction timeout values;
8. pass identical bucket/prefix values to this adapter and `s3blob`;
9. run contract tests against an isolated real PostgreSQL fixture before
   production enablement.

Do not dual-write SQLite and PostgreSQL. Cutover and rollback must follow the
one-way migration plan in `11-postgres-production-design.md`.

## 10. Tests in this stage

The package tests use a standard-library-only scripted `database/sql` driver.
No real PostgreSQL driver or RDS connection is required. Tests cover:

- PostgreSQL placeholder generation and tenant-first arguments;
- escaped free-text filters;
- deterministic pagination order;
- `BIGINT` boundary values;
- UTC normalization;
- cross-tenant `ErrNotFound`;
- object ordering;
- atomic pending-object query sequence;
- idempotency payload mismatch;
- status CAS conflict versus missing/cross-tenant behavior;
- portable PostgreSQL SQLSTATE conflict mapping;
- invalid date/range/offset validation.
- complete product-registration query order in one transaction;
- normalized accessory and box relation writes;
- concurrency-safe product-code allocation and exhaustion;
- stored idempotency replay of product ID/code/version;
- cross-tenant supplier rejection;
- pre-transaction rejection of invalid product status, number, and duplicate
  accessories.
- complete query order for purchase, exempt-JPY sale, shipment-allocation, and
  single-item inventory-restoration workflow transactions;
- workflow status/version compare-and-swap updates, inventory events, audit
  rows, idempotency result persistence, and yearly business-number formats;
- pre-transaction rejection of unresolved foreign-currency and multi-item
  restore semantics.

## 11. Deferred work

- Register and configure the chosen PostgreSQL driver in the composition root.
- Add real PostgreSQL integration/contract tests in an isolated environment.
- Resolve the workflow DTO/DDL gaps listed in section 5.2.1 before expanding
  the deliberately restricted workflow behavior.
- Add RLS only after transaction-scoped tenant session variables and pool reset
  behavior are proven.
- Add metrics/tracing outside returned diagnostic text.
- Run RDS load, failover, serialization-retry, PITR, and S3 reconciliation
  drills before cutover.
