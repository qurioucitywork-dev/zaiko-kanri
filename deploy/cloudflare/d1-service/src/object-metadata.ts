export interface ObjectMetadataEnv {
  DB: D1Database;
}

type ObjectStatus = "pending" | "ready" | "failed" | "deleted";

interface ObjectRow {
  id: string;
  organization_id: string;
  product_id: string;
  checksum_sha256: string;
  original_name: string;
  content_type: string;
  size_bytes: number;
  sort_order: number;
  status: ObjectStatus;
  created_at: string;
  ready_at: string | null;
  deleted_at: string | null;
}

interface IdempotencyRow {
  canonical_hash: string;
  response_json: string;
}

interface CommandBody {
  actor_id: string;
  requested_at: string;
}

interface CreatePendingBody extends CommandBody {
  object_id: string;
  product_id: string;
  original_name: string;
  content_type: string;
  sort_order: number;
}

interface MarkReadyBody extends CommandBody {
  checksum_sha256: string;
  size_bytes: number;
  ready_at: string;
}

interface MarkFailedBody extends CommandBody {
  reason_code: string;
}

interface MarkDeletedBody extends CommandBody {
  deleted_at: string;
}

interface VerifiedCommand<T> {
  body: T;
  idempotencyKey: string;
  canonicalHash: string;
}

const objectSelect = `
SELECT id, organization_id, product_id, checksum_sha256, original_name,
       content_type, size_bytes, sort_order, status, created_at, ready_at,
       deleted_at
  FROM object_metadata
 WHERE organization_id = ? AND id = ?`;

function json(body: unknown, status = 200): Response {
  return Response.json(body, {
    status,
    headers: { "cache-control": "no-store" },
  });
}

function problem(code: string, status: number): Response {
  return json({ error: code }, status);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function requiredString(
  value: unknown,
  maxLength: number,
): value is string {
  return (
    typeof value === "string" &&
    value.trim().length > 0 &&
    value.length <= maxLength
  );
}

function validTimestamp(value: unknown): value is string {
  return (
    requiredString(value, 64) &&
    Number.isFinite(Date.parse(value))
  );
}

function validCommandBody(
  value: unknown,
): value is Record<string, unknown> & CommandBody {
  return (
    isRecord(value) &&
    requiredString(value.actor_id, 128) &&
    validTimestamp(value.requested_at)
  );
}

function validObjectID(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^[A-Za-z0-9_-]{1,128}$/.test(value)
  );
}

function validContentType(value: unknown): value is string {
  return (
    value === "image/jpeg" ||
    value === "image/png" ||
    value === "image/webp"
  );
}

function validSHA256(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
}

function validReasonCode(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^[a-z][a-z0-9_]{0,63}$/.test(value)
  );
}

function validCreatePending(value: unknown): value is CreatePendingBody {
  return (
    validCommandBody(value) &&
    validObjectID(value.object_id) &&
    requiredString(value.product_id, 128) &&
    requiredString(value.original_name, 512) &&
    validContentType(value.content_type) &&
    Number.isSafeInteger(value.sort_order) &&
    Number(value.sort_order) >= 0
  );
}

function validMarkReady(value: unknown): value is MarkReadyBody {
  return (
    validCommandBody(value) &&
    validSHA256(value.checksum_sha256) &&
    Number.isSafeInteger(value.size_bytes) &&
    Number(value.size_bytes) > 0 &&
    validTimestamp(value.ready_at)
  );
}

function validMarkFailed(value: unknown): value is MarkFailedBody {
  return validCommandBody(value) && validReasonCode(value.reason_code);
}

function validMarkDeleted(value: unknown): value is MarkDeletedBody {
  return validCommandBody(value) && validTimestamp(value.deleted_at);
}

async function sha256Hex(value: string): Promise<string> {
  const bytes = new TextEncoder().encode(value);
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest))
    .map((part) => part.toString(16).padStart(2, "0"))
    .join("");
}

function canonicalObjectCommand(
  operationName: string,
  tenantID: string,
  body: Record<string, unknown> & CommandBody,
): string {
  const businessPayload: Record<string, unknown> = {};
  for (const key of Object.keys(body).sort()) {
    if (key !== "actor_id" && key !== "requested_at") {
      businessPayload[key] = body[key];
    }
  }
  const encoded = JSON.stringify({
    operation: operationName,
    tenant_id: tenantID.trim(),
    actor_id: body.actor_id.trim(),
    payload: businessPayload,
  });
  // Go's encoding/json always escapes U+2028/U+2029. Mirror that behavior;
  // Go disables its optional HTML escaping so <, >, and & remain literal,
  // exactly as JSON.stringify emits them.
  return encoded.replace(
    /[\u2028\u2029]/g,
    (character) => character === "\u2028" ? "\\u2028" : "\\u2029",
  );
}

async function verifiedCommand<T>(
  request: Request,
  tenantID: string,
  operationName: string,
  validator: (value: unknown) => value is T,
): Promise<VerifiedCommand<T> | Response> {
  const idempotencyKey =
    request.headers.get("x-zaiko-idempotency-key")?.trim() ?? "";
  const suppliedHash =
    request.headers.get("x-zaiko-canonical-hash")?.trim().toLowerCase() ?? "";
  if (
    !requiredString(idempotencyKey, 128) ||
    !validSHA256(suppliedHash)
  ) {
    return problem("invalid_argument", 400);
  }

  const raw = await request.text();
  if (raw.length === 0 || raw.length > 64 * 1024) {
    return problem("invalid_argument", 400);
  }
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    return problem("invalid_argument", 400);
  }
  if (!validator(value)) {
    return problem("invalid_argument", 400);
  }
  const computedHash = await sha256Hex(canonicalObjectCommand(
    operationName,
    tenantID,
    value as Record<string, unknown> & CommandBody,
  ));
  if (computedHash !== suppliedHash) {
    return problem("invalid_argument", 400);
  }
  return {
    body: value,
    idempotencyKey,
    canonicalHash: computedHash,
  };
}

function wireObject(row: ObjectRow): Record<string, unknown> {
  return {
    id: row.id,
    organization_id: row.organization_id,
    product_id: row.product_id,
    checksum_sha256: row.checksum_sha256,
    original_name: row.original_name,
    content_type: row.content_type,
    size_bytes: String(row.size_bytes),
    sort_order: row.sort_order,
    status: row.status,
    created_at: row.created_at,
    ready_at: row.ready_at ?? "",
    deleted_at: row.deleted_at ?? "",
  };
}

async function replay(
  env: ObjectMetadataEnv,
  tenantID: string,
  operationName: string,
  command: VerifiedCommand<unknown>,
  empty: boolean,
): Promise<Response | null> {
  const row = await env.DB.prepare(
    `SELECT canonical_hash, response_json
       FROM object_metadata_idempotency
      WHERE organization_id = ? AND operation_name = ?
        AND idempotency_key = ?`,
  )
    .bind(tenantID, operationName, command.idempotencyKey)
    .first<IdempotencyRow>();
  if (!row) {
    return null;
  }
  if (row.canonical_hash !== command.canonicalHash) {
    return problem("idempotency_mismatch", 409);
  }
  if (empty) {
    return new Response(null, {
      status: 204,
      headers: { "cache-control": "no-store" },
    });
  }
  return new Response(row.response_json, {
    status: 200,
    headers: {
      "cache-control": "no-store",
      "content-type": "application/json",
    },
  });
}

async function actorAndProductExist(
  env: ObjectMetadataEnv,
  tenantID: string,
  actorID: string,
  productID: string,
): Promise<boolean> {
  const row = await env.DB.prepare(
    `SELECT
       EXISTS(
         SELECT 1 FROM users u
         JOIN organizations o ON o.id = u.organization_id
          WHERE u.organization_id = ? AND u.id = ?
            AND o.is_active = 1
            AND u.is_active = 1 AND u.deleted_at IS NULL
            AND (
              EXISTS(
                SELECT 1 FROM user_permissions up
                 WHERE up.user_id = u.id
                   AND up.permission_key = 'inventory.write'
                   AND up.effect = 'allow'
              )
              OR (
                NOT EXISTS(
                  SELECT 1 FROM user_permissions up
                   WHERE up.user_id = u.id
                     AND up.permission_key = 'inventory.write'
                )
                AND EXISTS(
                  SELECT 1 FROM role_permissions rp
                   WHERE rp.role_key = u.role_key
                     AND rp.permission_key = 'inventory.write'
                )
              )
            )
       ) AS actor_ok,
       EXISTS(
         SELECT 1 FROM products
          WHERE organization_id = ? AND id = ? AND deleted_at IS NULL
       ) AS product_ok`,
  )
    .bind(tenantID, actorID, tenantID, productID)
    .first<{ actor_ok: number; product_ok: number }>();
  return row?.actor_ok === 1 && row.product_ok === 1;
}

async function scopedObjectAndActor(
  env: ObjectMetadataEnv,
  tenantID: string,
  objectID: string,
  actorID: string,
): Promise<(ObjectRow & { actor_ok: number }) | null> {
  return env.DB.prepare(
    `SELECT id, organization_id, product_id, checksum_sha256, original_name,
            content_type, size_bytes, sort_order, status, created_at, ready_at,
            deleted_at,
            EXISTS(
              SELECT 1 FROM users u
              JOIN organizations o ON o.id = u.organization_id
               WHERE u.organization_id = ? AND u.id = ?
                 AND o.is_active = 1
                 AND u.is_active = 1 AND u.deleted_at IS NULL
                 AND (
                   EXISTS(
                     SELECT 1 FROM user_permissions up
                      WHERE up.user_id = u.id
                        AND up.permission_key = 'inventory.write'
                        AND up.effect = 'allow'
                   )
                   OR (
                     NOT EXISTS(
                       SELECT 1 FROM user_permissions up
                        WHERE up.user_id = u.id
                          AND up.permission_key = 'inventory.write'
                     )
                     AND EXISTS(
                       SELECT 1 FROM role_permissions rp
                        WHERE rp.role_key = u.role_key
                          AND rp.permission_key = 'inventory.write'
                     )
                   )
                 )
            ) AS actor_ok
       FROM object_metadata
      WHERE organization_id = ? AND id = ?`,
  )
    .bind(tenantID, actorID, tenantID, objectID)
    .first<ObjectRow & { actor_ok: number }>();
}

export async function createPendingObject(
  request: Request,
  env: ObjectMetadataEnv,
  tenantID: string,
): Promise<Response> {
  if (!requiredString(tenantID, 128)) {
    return problem("invalid_argument", 400);
  }
  const command = await verifiedCommand(
    request,
    tenantID,
    "object.create_pending",
    validCreatePending,
  );
  if (command instanceof Response) {
    return command;
  }
  const body = command.body;
  if (
    !(await actorAndProductExist(
      env,
      tenantID,
      body.actor_id,
      body.product_id,
    ))
  ) {
    return problem("not_found", 404);
  }
  const replayed = await replay(
    env,
    tenantID,
    "object.create_pending",
    command,
    false,
  );
  if (replayed) {
    return replayed;
  }

  const row: ObjectRow = {
    id: body.object_id,
    organization_id: tenantID,
    product_id: body.product_id,
    checksum_sha256: "",
    original_name: body.original_name,
    content_type: body.content_type,
    size_bytes: 0,
    sort_order: body.sort_order,
    status: "pending",
    created_at: body.requested_at,
    ready_at: null,
    deleted_at: null,
  };
  const responseJSON = JSON.stringify({ object: wireObject(row) });
  try {
    await env.DB.batch([
      env.DB.prepare(
        `INSERT INTO object_metadata (
           organization_id, id, product_id, checksum_sha256, original_name,
           content_type, size_bytes, sort_order, status, failure_reason_code,
           created_by, created_at, updated_at, ready_at, deleted_at
         ) VALUES (?, ?, ?, '', ?, ?, 0, ?, 'pending', '', ?, ?, ?, NULL, NULL)`,
      ).bind(
        tenantID,
        body.object_id,
        body.product_id,
        body.original_name,
        body.content_type,
        body.sort_order,
        body.actor_id,
        body.requested_at,
        body.requested_at,
      ),
      env.DB.prepare(
        `INSERT INTO object_metadata_idempotency (
           organization_id, operation_name, idempotency_key, canonical_hash,
           object_id, actor_id, response_json, created_at
         ) VALUES (?, 'object.create_pending', ?, ?, ?, ?, ?, ?)`,
      ).bind(
        tenantID,
        command.idempotencyKey,
        command.canonicalHash,
        body.object_id,
        body.actor_id,
        responseJSON,
        body.requested_at,
      ),
    ]);
  } catch {
    const concurrentReplay = await replay(
      env,
      tenantID,
      "object.create_pending",
      command,
      false,
    );
    return concurrentReplay ?? problem("conflict", 409);
  }
  return new Response(responseJSON, {
    status: 201,
    headers: {
      "cache-control": "no-store",
      "content-type": "application/json",
    },
  });
}

async function transitionObject<T extends CommandBody>(
  env: ObjectMetadataEnv,
  tenantID: string,
  objectID: string,
  operationName: string,
  command: VerifiedCommand<T>,
  update: D1PreparedStatement,
  responseJSON: string,
  empty: boolean,
): Promise<Response> {
  const results = await env.DB.batch([
    update,
    env.DB.prepare(
      `INSERT INTO object_metadata_idempotency (
         organization_id, operation_name, idempotency_key, canonical_hash,
         object_id, actor_id, response_json, created_at
       )
       SELECT ?, ?, ?, ?, ?, ?, ?, ?
        WHERE changes() = 1`,
    ).bind(
      tenantID,
      operationName,
      command.idempotencyKey,
      command.canonicalHash,
      objectID,
      command.body.actor_id,
      responseJSON,
      command.body.requested_at,
    ),
  ]);
  const changed = Number(results[0]?.meta.changes ?? 0);
  if (changed !== 1) {
    const concurrentReplay = await replay(
      env,
      tenantID,
      operationName,
      command,
      empty,
    );
    return concurrentReplay ?? problem("conflict", 409);
  }
  if (empty) {
    return new Response(null, {
      status: 204,
      headers: { "cache-control": "no-store" },
    });
  }
  return new Response(responseJSON, {
    status: 200,
    headers: {
      "cache-control": "no-store",
      "content-type": "application/json",
    },
  });
}

export async function markObjectReady(
  request: Request,
  env: ObjectMetadataEnv,
  tenantID: string,
  objectID: string,
): Promise<Response> {
  if (!requiredString(tenantID, 128) || !validObjectID(objectID)) {
    return problem("invalid_argument", 400);
  }
  const command = await verifiedCommand(
    request,
    tenantID,
    "object.mark_ready",
    validMarkReady,
  );
  if (command instanceof Response) {
    return command;
  }
  const current = await scopedObjectAndActor(
    env,
    tenantID,
    objectID,
    command.body.actor_id,
  );
  if (!current || current.actor_ok !== 1) {
    return problem("not_found", 404);
  }
  const replayed = await replay(
    env,
    tenantID,
    "object.mark_ready",
    command,
    false,
  );
  if (replayed) {
    return replayed;
  }
  if (current.status !== "pending") {
    return problem("conflict", 409);
  }
  const ready: ObjectRow = {
    ...current,
    checksum_sha256: command.body.checksum_sha256,
    size_bytes: command.body.size_bytes,
    status: "ready",
    ready_at: command.body.ready_at,
  };
  const responseJSON = JSON.stringify({ object: wireObject(ready) });
  try {
    return await transitionObject(
      env,
      tenantID,
      objectID,
      "object.mark_ready",
      command,
      env.DB.prepare(
        `UPDATE object_metadata
            SET checksum_sha256 = ?, size_bytes = ?, status = 'ready',
                ready_at = ?, updated_at = ?
          WHERE organization_id = ? AND id = ? AND status = 'pending'`,
      ).bind(
        command.body.checksum_sha256,
        command.body.size_bytes,
        command.body.ready_at,
        command.body.requested_at,
        tenantID,
        objectID,
      ),
      responseJSON,
      false,
    );
  } catch {
    const concurrentReplay = await replay(
      env,
      tenantID,
      "object.mark_ready",
      command,
      false,
    );
    return concurrentReplay ?? problem("conflict", 409);
  }
}

export async function markObjectFailed(
  request: Request,
  env: ObjectMetadataEnv,
  tenantID: string,
  objectID: string,
): Promise<Response> {
  if (!requiredString(tenantID, 128) || !validObjectID(objectID)) {
    return problem("invalid_argument", 400);
  }
  const command = await verifiedCommand(
    request,
    tenantID,
    "object.mark_failed",
    validMarkFailed,
  );
  if (command instanceof Response) {
    return command;
  }
  const current = await scopedObjectAndActor(
    env,
    tenantID,
    objectID,
    command.body.actor_id,
  );
  if (!current || current.actor_ok !== 1) {
    return problem("not_found", 404);
  }
  const replayed = await replay(
    env,
    tenantID,
    "object.mark_failed",
    command,
    true,
  );
  if (replayed) {
    return replayed;
  }
  if (current.status !== "pending") {
    return problem("conflict", 409);
  }
  try {
    return await transitionObject(
      env,
      tenantID,
      objectID,
      "object.mark_failed",
      command,
      env.DB.prepare(
        `UPDATE object_metadata
            SET status = 'failed', failure_reason_code = ?, updated_at = ?
          WHERE organization_id = ? AND id = ? AND status = 'pending'`,
      ).bind(
        command.body.reason_code,
        command.body.requested_at,
        tenantID,
        objectID,
      ),
      "{}",
      true,
    );
  } catch {
    const concurrentReplay = await replay(
      env,
      tenantID,
      "object.mark_failed",
      command,
      true,
    );
    return concurrentReplay ?? problem("conflict", 409);
  }
}

export async function markObjectDeleted(
  request: Request,
  env: ObjectMetadataEnv,
  tenantID: string,
  objectID: string,
): Promise<Response> {
  if (!requiredString(tenantID, 128) || !validObjectID(objectID)) {
    return problem("invalid_argument", 400);
  }
  const command = await verifiedCommand(
    request,
    tenantID,
    "object.mark_deleted",
    validMarkDeleted,
  );
  if (command instanceof Response) {
    return command;
  }
  const current = await scopedObjectAndActor(
    env,
    tenantID,
    objectID,
    command.body.actor_id,
  );
  if (!current || current.actor_ok !== 1) {
    return problem("not_found", 404);
  }
  const replayed = await replay(
    env,
    tenantID,
    "object.mark_deleted",
    command,
    true,
  );
  if (replayed) {
    return replayed;
  }
  if (current.status === "deleted") {
    return problem("conflict", 409);
  }
  try {
    return await transitionObject(
      env,
      tenantID,
      objectID,
      "object.mark_deleted",
      command,
      env.DB.prepare(
        `UPDATE object_metadata
            SET status = 'deleted', deleted_at = ?, updated_at = ?
          WHERE organization_id = ? AND id = ?
            AND status IN ('pending', 'ready', 'failed')`,
      ).bind(
        command.body.deleted_at,
        command.body.requested_at,
        tenantID,
        objectID,
      ),
      "{}",
      true,
    );
  } catch {
    const concurrentReplay = await replay(
      env,
      tenantID,
      "object.mark_deleted",
      command,
      true,
    );
    return concurrentReplay ?? problem("conflict", 409);
  }
}

// handleObjectMetadataRequest is the only integration point index.ts needs.
// A null result means the path does not belong to this module.
export async function handleObjectMetadataRequest(
  request: Request,
  env: ObjectMetadataEnv,
  tenantID: string,
): Promise<Response | null> {
  if (request.method !== "POST") {
    return null;
  }
  const path = new URL(request.url).pathname;
  if (path === "/internal/v1/object-metadata/pending") {
    return createPendingObject(request, env, tenantID);
  }
  const match = path.match(
    /^\/internal\/v1\/object-metadata\/([^/]+)\/(ready|failed|deleted)$/,
  );
  if (!match) {
    return null;
  }
  let objectID: string;
  try {
    objectID = decodeURIComponent(match[1]);
  } catch {
    return problem("invalid_argument", 400);
  }
  switch (match[2]) {
    case "ready":
      return markObjectReady(request, env, tenantID, objectID);
    case "failed":
      return markObjectFailed(request, env, tenantID, objectID);
    case "deleted":
      return markObjectDeleted(request, env, tenantID, objectID);
    default:
      return null;
  }
}
