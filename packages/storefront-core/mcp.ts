import { FRONT_MCP_URL } from "./backend-url";
import { clientIP } from "./client-ip";
import { request } from "./request";

// The protocol's own headers - cookies and credentials stay behind, since the endpoint answers every agent the same
const FORWARDED = ["content-type", "accept", "mcp-protocol-version", "mcp-session-id", "last-event-id"];

// The storefront's /api/mcp: a shopper's AI agent talks to this store's own channel, whatever it sends
export async function proxyMcp(req: Request, backend: string = FRONT_MCP_URL): Promise<Response> {
  const headers = new Headers({ "X-Channel-Id": await request().channelId() });
  for (const h of FORWARDED) {
    const v = req.headers.get(h);
    if (v) headers.set(h, v);
  }
  const ip = clientIP(req.headers);
  if (ip) headers.set("X-Forwarded-For", ip);
  let res: Response;
  try {
    res = await fetch(backend, {
      method: req.method,
      headers,
      body: req.method === "POST" ? await req.arrayBuffer() : undefined,
      cache: "no-store",
    });
  } catch {
    return new Response("Bad gateway", { status: 502 });
  }
  const out = new Headers();
  for (const h of ["content-type", "allow", "mcp-session-id", "cache-control"]) {
    const v = res.headers.get(h);
    if (v) out.set(h, v);
  }
  return new Response(res.body, { status: res.status, headers: out });
}
