import { handleObjectMetadataRequest } from "./object-metadata";

interface Env {
  DB: D1Database;
}

type BindValue = string | number | null;

function json(body: unknown, status = 200): Response {
  return Response.json(body, {
    status,
    headers: { "cache-control": "no-store" },
  });
}

function requireInternal(request: Request): Response | null {
  if (
    request.headers.get("x-zaiko-internal-hop") !== "container-outbound" ||
    !request.headers.get("x-zaiko-container-id")
  ) {
    return json({ error: "not_found" }, 404);
  }
  return null;
}

function requireTenant(request: Request): string | Response {
  const tenantID = request.headers.get("x-zaiko-tenant-id")?.trim() ?? "";
  if (!tenantID || tenantID.length > 128) {
    return json({ error: "invalid_argument" }, 400);
  }
  return tenantID;
}

function positiveInt(raw: string | null, fallback: number, max: number): number {
  const parsed = Number.parseInt(raw ?? "", 10);
  if (!Number.isSafeInteger(parsed) || parsed < 1) {
    return fallback;
  }
  return Math.min(parsed, max);
}

const productSelect = `
SELECT
  p.id,
  p.organization_id,
  p.product_code,
  p.sku,
  p.brand,
  p.model_number,
  p.serial_number,
  p.product_type,
  p.supplier_id,
  COALESCE(s.name, '') AS supplier_name,
  COALESCE(ps.created_by, '') AS buyer_id,
  COALESCE(u.display_name, '') AS buyer_name,
  p.purchase_date,
  CAST(p.cost_amount_minor AS TEXT) AS cost_amount_minor,
  p.cost_currency,
  CAST(p.base_sale_price_minor AS TEXT) AS base_sale_price_minor,
  p.base_sale_currency,
  p.inventory_status,
  p.publication_status,
  p.condition_text,
  p.accessories,
  p.material_text,
  p.box_text,
  p.movement_text,
  p.belt_material_text,
  p.dial_text,
  p.features_text,
  p.created_at,
  (SELECT COUNT(*) FROM product_images pi
    WHERE pi.organization_id = p.organization_id AND pi.product_id = p.id) AS image_count
FROM products p
LEFT JOIN suppliers s
  ON s.organization_id = p.organization_id AND s.id = p.supplier_id
LEFT JOIN purchase_slip_lines psl
  ON psl.id = p.purchase_slip_line_id
LEFT JOIN purchase_slips ps
  ON ps.organization_id = p.organization_id AND ps.id = psl.purchase_slip_id
LEFT JOIN users u
  ON u.organization_id = p.organization_id AND u.id = ps.created_by
`;

async function health(env: Env): Promise<Response> {
  try {
    const row = await env.DB.prepare("SELECT 1 AS ok").first<{ ok: number }>();
    return json({ provider: "d1", status: row?.ok === 1 ? "ok" : "failed" });
  } catch {
    return json({ provider: "d1", status: "failed" }, 503);
  }
}

async function getProduct(
  env: Env,
  tenantID: string,
  productID: string,
): Promise<Response> {
  const row = await env.DB.prepare(
    `${productSelect} WHERE p.organization_id = ? AND p.id = ? AND p.deleted_at IS NULL`,
  )
    .bind(tenantID, productID)
    .first();
  return row ? json({ product: row }) : json({ error: "not_found" }, 404);
}

async function searchProducts(
  request: Request,
  env: Env,
  tenantID: string,
): Promise<Response> {
  const url = new URL(request.url);
  const page = positiveInt(url.searchParams.get("page"), 1, 1_000_000);
  const pageSize = positiveInt(url.searchParams.get("page_size"), 50, 200);
  const clauses = ["p.organization_id = ?", "p.deleted_at IS NULL"];
  const values: BindValue[] = [tenantID];

  const exactFilters: Array<[string, string]> = [
    ["brand", "p.brand"],
    ["supplier_id", "p.supplier_id"],
    ["inventory_status", "p.inventory_status"],
  ];
  for (const [parameter, column] of exactFilters) {
    const value = url.searchParams.get(parameter)?.trim();
    if (value) {
      clauses.push(`${column} = ?`);
      values.push(value);
    }
  }

  const dateFrom = url.searchParams.get("purchase_date_from")?.trim();
  if (dateFrom) {
    clauses.push("p.purchase_date >= ?");
    values.push(dateFrom);
  }
  const dateTo = url.searchParams.get("purchase_date_to")?.trim();
  if (dateTo) {
    clauses.push("p.purchase_date <= ?");
    values.push(dateTo);
  }
  const query = url.searchParams.get("query")?.trim();
  if (query) {
    clauses.push(
      "(p.product_code LIKE ? ESCAPE '\\' OR p.sku LIKE ? ESCAPE '\\' OR p.brand LIKE ? ESCAPE '\\' OR p.model_number LIKE ? ESCAPE '\\' OR p.serial_number LIKE ? ESCAPE '\\')",
    );
    const pattern = `%${query.replaceAll("\\", "\\\\").replaceAll("%", "\\%").replaceAll("_", "\\_")}%`;
    values.push(pattern, pattern, pattern, pattern, pattern);
  }

  const where = clauses.join(" AND ");
  const countRow = await env.DB.prepare(
    `SELECT COUNT(*) AS total FROM products p WHERE ${where}`,
  )
    .bind(...values)
    .first<{ total: number }>();
  const total = Number(countRow?.total ?? 0);
  const offset = (page - 1) * pageSize;
  const result = await env.DB.prepare(
    `${productSelect} WHERE ${where}
     ORDER BY p.purchase_date DESC, p.product_code ASC
     LIMIT ? OFFSET ?`,
  )
    .bind(...values, pageSize, offset)
    .all();

  return json({
    items: result.results,
    total,
    page,
    page_size: pageSize,
    total_pages: total === 0 ? 0 : Math.ceil(total / pageSize),
  });
}

async function listProductObjects(
  env: Env,
  tenantID: string,
  productID: string,
): Promise<Response> {
  const product = await env.DB.prepare(
    "SELECT 1 AS ok FROM products WHERE organization_id = ? AND id = ? AND deleted_at IS NULL",
  )
    .bind(tenantID, productID)
    .first();
  if (!product) {
    return json({ error: "not_found" }, 404);
  }
  const rows = await env.DB.prepare(
    `SELECT id, organization_id, product_id, checksum_sha256,
            original_name, content_type, CAST(size_bytes AS TEXT) AS size_bytes,
            sort_order, status, created_at,
            COALESCE(ready_at, '') AS ready_at,
            COALESCE(deleted_at, '') AS deleted_at
       FROM object_metadata
      WHERE organization_id = ? AND product_id = ?
      UNION ALL
     SELECT pi.id, pi.organization_id, pi.product_id, '' AS checksum_sha256,
            pi.original_name, pi.content_type,
            CAST(pi.size_bytes AS TEXT) AS size_bytes,
            pi.sort_order, 'ready' AS status, pi.created_at,
            pi.created_at AS ready_at, '' AS deleted_at
       FROM product_images pi
      WHERE pi.organization_id = ? AND pi.product_id = ?
        AND NOT EXISTS (
          SELECT 1 FROM object_metadata om
           WHERE om.organization_id = pi.organization_id AND om.id = pi.id
        )
      ORDER BY sort_order ASC, id ASC`,
  )
    .bind(tenantID, productID, tenantID, productID)
    .all();
  return json({ objects: rows.results });
}

async function getObject(
  env: Env,
  tenantID: string,
  objectID: string,
): Promise<Response> {
  const row = await env.DB.prepare(
    `SELECT id, organization_id, product_id, checksum_sha256,
            original_name, content_type, CAST(size_bytes AS TEXT) AS size_bytes,
            sort_order, status, created_at,
            COALESCE(ready_at, '') AS ready_at,
            COALESCE(deleted_at, '') AS deleted_at
       FROM object_metadata
      WHERE organization_id = ? AND id = ?
      UNION ALL
     SELECT pi.id, pi.organization_id, pi.product_id, '' AS checksum_sha256,
            pi.original_name, pi.content_type,
            CAST(pi.size_bytes AS TEXT) AS size_bytes,
            pi.sort_order, 'ready' AS status, pi.created_at,
            pi.created_at AS ready_at, '' AS deleted_at
       FROM product_images pi
      WHERE pi.organization_id = ? AND pi.id = ?
        AND NOT EXISTS (
          SELECT 1 FROM object_metadata om
           WHERE om.organization_id = pi.organization_id AND om.id = pi.id
        )
      LIMIT 1`,
  )
    .bind(tenantID, objectID, tenantID, objectID)
    .first();
  return row ? json({ object: row }) : json({ error: "not_found" }, 404);
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    try {
      const internalError = requireInternal(request);
      if (internalError) {
        return internalError;
      }

      const url = new URL(request.url);
      if (url.pathname === "/internal/v1/diagnostics") {
        if (request.method !== "GET") {
          return json({ error: "method_not_allowed" }, 405);
        }
        return health(env);
      }

      const tenant = requireTenant(request);
      if (tenant instanceof Response) {
        return tenant;
      }

      const objectMetadataResponse = await handleObjectMetadataRequest(
        request,
        env,
        tenant,
      );
      if (objectMetadataResponse) {
        return objectMetadataResponse;
      }
      if (request.method !== "GET") {
        return json({ error: "method_not_allowed" }, 405);
      }

      if (url.pathname === "/internal/v1/products") {
        return searchProducts(request, env, tenant);
      }
      const productMatch = url.pathname.match(/^\/internal\/v1\/products\/([^/]+)$/);
      if (productMatch) {
        return getProduct(env, tenant, decodeURIComponent(productMatch[1]));
      }
      const objectListMatch = url.pathname.match(
        /^\/internal\/v1\/products\/([^/]+)\/objects$/,
      );
      if (objectListMatch) {
        return listProductObjects(env, tenant, decodeURIComponent(objectListMatch[1]));
      }
      const objectMatch = url.pathname.match(/^\/internal\/v1\/objects\/([^/]+)$/);
      if (objectMatch) {
        return getObject(env, tenant, decodeURIComponent(objectMatch[1]));
      }
      return json({ error: "not_found" }, 404);
    } catch {
      // Never expose D1, binding, or SQL details to the Container.
      return json({ error: "service_unavailable" }, 503);
    }
  },
} satisfies ExportedHandler<Env>;
