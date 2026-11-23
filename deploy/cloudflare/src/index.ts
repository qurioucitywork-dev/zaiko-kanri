import { DurableObject } from "cloudflare:workers";
import { storageGateway } from "./storage-gateway";

export interface Env {
  APP: DurableObjectNamespace<ZaikoContainer>;
  IMAGES: R2Bucket;
  DATABASE_URL: string;
  DATABASE_MIGRATION_URL: string;
  R2_GATEWAY_TOKEN: string;
  ADMIN_PASSWORD: string;
  WORKER_PASSWORD: string;
  PUBLIC_BASE_URL: string;
  ORGANIZATION_CODE: string;
}
const PORT = 8080;
// The existing Go process runs SMBC synchronization every 30 minutes. Cron keeps
// this singleton alive so its scheduler behaves like the existing Docker app.
const IDLE_MS = 40 * 60 * 1000;

function containerEnvironment(env: Env): Record<string, string> {
  for (const key of ["DATABASE_URL", "DATABASE_MIGRATION_URL", "R2_GATEWAY_TOKEN", "ADMIN_PASSWORD", "WORKER_PASSWORD"] as const) {
    if (!env[key]) throw new Error(`Missing required secret: ${key}`);
  }
  const base = new URL(env.PUBLIC_BASE_URL);
  if (base.protocol !== "https:" || base.hostname.includes("replace")) throw new Error("Configure the HTTPS PUBLIC_BASE_URL before deployment");
  if (env.R2_GATEWAY_TOKEN.length < 32 || env.ADMIN_PASSWORD.length < 16 || env.WORKER_PASSWORD.length < 16) throw new Error("Deployment secrets are too short");
  return {
    ZAIKO_ADDRESS: "0.0.0.0:8080", ZAIKO_ENV: "production", ZAIKO_COOKIE_SECURE: "true",
    ZAIKO_PUBLIC_DEMO_LOGIN: "false", ZAIKO_PUBLIC_BASE_URL: base.origin,
    ZAIKO_ORGANIZATION_CODE: env.ORGANIZATION_CODE,
    ZAIKO_DATABASE_DRIVER: "postgres", ZAIKO_DATABASE_URL: env.DATABASE_URL,
    ZAIKO_DATABASE_MIGRATION_URL: env.DATABASE_MIGRATION_URL,
    ZAIKO_DATABASE_PATH: "/tmp/zaiko-compat.db", ZAIKO_UPLOAD_DIRECTORY: "/tmp/uploads",
    ZAIKO_STORAGE_DRIVER: "r2gateway", ZAIKO_R2_GATEWAY_URL: `${base.origin}/__storage`,
    ZAIKO_R2_GATEWAY_TOKEN: env.R2_GATEWAY_TOKEN,
    ZAIKO_PREVIEW_ADMIN_PASSWORD: env.ADMIN_PASSWORD,
    ZAIKO_PREVIEW_WORKER_PASSWORD: env.WORKER_PASSWORD,
    ZAIKO_SMBC_EXCHANGE_RATES: "true",
  };
}

export class ZaikoContainer extends DurableObject<Env> {
  private starting?: Promise<void>;
  private monitoring?: Promise<void>;
  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    if (ctx.container?.running) {
      void ctx.blockConcurrencyWhile(() => ctx.container!.setInactivityTimeout(IDLE_MS));
      this.observeExit();
    }
  }
  async fetch(request: Request): Promise<Response> {
    this.starting ??= this.ensureReady().finally(() => { this.starting = undefined; });
    await this.starting;
    const url = new URL(request.url);
    url.protocol = "http:";
    url.host = "container";
    const forwarded = new Request(url, request);
    forwarded.headers.delete("host");
    forwarded.headers.set("X-Forwarded-Proto", "https");
    // Mutating requests are forwarded exactly once; only readiness is retried.
    return this.ctx.container!.getTcpPort(PORT).fetch(forwarded);
  }
  private async ensureReady(): Promise<void> {
    const container = this.ctx.container!;
    if (!container.running) {
      container.start({ image: container.images.base, instance: "basic", enableInternet: true, env: containerEnvironment(this.env) });
    }
    this.observeExit();
    await container.setInactivityTimeout(IDLE_MS);
    for (let attempt = 0; attempt < 90; attempt++) {
      try {
        const response = await container.getTcpPort(PORT).fetch("http://container/healthz", { signal: AbortSignal.timeout(1000) });
        await response.body?.cancel();
        if (response.ok) return;
      } catch { /* Probe only; Neon startup/migrations may take a few seconds. */ }
      await scheduler.wait(500);
    }
    throw new Error("Application readiness timeout");
  }
  private observeExit(): void {
    if (this.monitoring) return;
    this.monitoring = this.ctx.container!.monitor()
      .then(() => { console.info("Application container stopped"); })
      .catch(() => { console.error("Application container exited unexpectedly"); })
      .finally(() => { this.monitoring = undefined; });
    this.ctx.waitUntil(this.monitoring);
  }
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname.startsWith("/__storage/")) return storageGateway(request, env);
    if (url.protocol !== "https:" && url.hostname !== "localhost") return Response.redirect(`https://${url.host}${url.pathname}${url.search}`, 308);
    try {
      return await env.APP.getByName("zaiko-singleton").fetch(request);
    } catch {
      return new Response("アプリを起動しています。少し待って再度アクセスしてください。", { status: 503, headers: { "Retry-After": "10", "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store" } });
    }
  },
  async scheduled(_controller: ScheduledController, env: Env): Promise<void> {
    const response = await env.APP.getByName("zaiko-singleton").fetch(new Request(`${env.PUBLIC_BASE_URL}/healthz`));
    await response.body?.cancel();
    if (!response.ok) throw new Error("Scheduled application health check failed");
  },
} satisfies ExportedHandler<Env>;

