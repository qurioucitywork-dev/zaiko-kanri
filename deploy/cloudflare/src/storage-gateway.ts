interface StorageEnv { IMAGES: R2Bucket; R2_GATEWAY_TOKEN: string }
const MAX_UPLOAD = 25 * 1024 * 1024;

async function authorized(request: Request, expected: string): Promise<boolean> {
  if (!expected || expected.length < 32) return false;
  const actual = request.headers.get("Authorization") || "";
  // Fixed-size digest comparison avoids leaking token prefix matches.
  const encoder = new TextEncoder();
  const [left, right] = await Promise.all([
    crypto.subtle.digest("SHA-256", encoder.encode(actual)),
    crypto.subtle.digest("SHA-256", encoder.encode(`Bearer ${expected}`)),
  ]);
  let difference = 0;
  const a = new Uint8Array(left), b = new Uint8Array(right);
  for (let i = 0; i < a.length; i++) difference |= a[i] ^ b[i];
  return difference === 0;
}

export async function storageGateway(request: Request, env: StorageEnv): Promise<Response> {
  const headers = { "Cache-Control": "no-store" };
  if (!(await authorized(request, env.R2_GATEWAY_TOKEN))) return new Response("Not found", { status: 404, headers });
  let key: string;
  try { key = decodeURIComponent(new URL(request.url).pathname.slice("/__storage/".length)); }
  catch { return new Response("Invalid key", { status: 400, headers }); }
  if (!key || key.length > 1024 || key.split("/").some(part => !part || part === "." || part === "..") || /[\\\x00-\x1f\x7f]/.test(key)) return new Response("Invalid key", { status: 400, headers });
  if (request.method === "GET") {
    const object = await env.IMAGES.get(key);
    if (!object) return new Response("Not found", { status: 404, headers });
    const responseHeaders = new Headers(headers);
    object.writeHttpMetadata(responseHeaders);
    responseHeaders.set("Content-Length", String(object.size));
    responseHeaders.set("X-Content-Type-Options", "nosniff");
    return new Response(object.body, { headers: responseHeaders });
  }
  if (request.method === "PUT") {
    const declared = Number(request.headers.get("Content-Length") || 0);
    if (declared > MAX_UPLOAD) return new Response("Too large", { status: 413, headers });
    if (!request.body) return new Response("Body required", { status: 400, headers });
    const reader = request.body.getReader();
    const chunks: Uint8Array[] = [];
    let size = 0;
    while (true) {
      const result = await reader.read();
      if (result.done) break;
      size += result.value.byteLength;
      if (size > MAX_UPLOAD) { await reader.cancel(); return new Response("Too large", { status: 413, headers }); }
      chunks.push(result.value);
    }
    const bytes = new Uint8Array(size);
    let offset = 0;
    for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
    await env.IMAGES.put(key, bytes, { httpMetadata: { contentType: request.headers.get("Content-Type") || "application/octet-stream" } });
    return Response.json({ size }, { headers });
  }
  if (request.method === "DELETE") {
    await env.IMAGES.delete(key);
    return new Response(null, { status: 204, headers });
  }
  return new Response("Method not allowed", { status: 405, headers: { ...headers, Allow: "GET, PUT, DELETE" } });
}
