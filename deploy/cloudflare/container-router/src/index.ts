import { Container, type OutboundHandlerContext } from "@cloudflare/containers";

export { ContainerProxy } from "@cloudflare/containers";

interface Env {
  ZAIKO_CONTAINER: DurableObjectNamespace<ZaikoContainer>;
  D1_SERVICE: Fetcher;
  ZAIKO_CONTAINER_INSTANCE: string;
}

export class ZaikoContainer extends Container<Env> {
  defaultPort = 8080;
  sleepAfter = "10m";
  enableInternet = false;
  allowedHosts = ["d1.internal"];
}

ZaikoContainer.outboundByHost = {
  "d1.internal": async (request: Request, env: Env, ctx: OutboundHandlerContext) => {
    const forwarded = new Request(request);
    forwarded.headers.set("x-zaiko-container-id", ctx.containerId);
    forwarded.headers.set("x-zaiko-internal-hop", "container-outbound");
    return env.D1_SERVICE.fetch(forwarded);
  },
};

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/edge-healthz") {
      return Response.json({ status: "ok", component: "container-router" });
    }

    const instanceName = env.ZAIKO_CONTAINER_INSTANCE || "preview-singleton";
    const container = env.ZAIKO_CONTAINER.getByName(instanceName);
    return container.fetch(request);
  },
} satisfies ExportedHandler<Env>;
