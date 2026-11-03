import { FRONT_MCP_URL } from "./backend-url";
import { clientIP } from "./client-ip";
import { request } from "./request";
import { boundedText } from "./revalidate-payload";

// The backend refuses anything larger, so there's no reason to hold more of an anonymous body than this
const MAX_BODY_BYTES = 64 * 1024;
// A little past the backend's own per-call limit, so it's the one that answers a slow tool
const TIMEOUT_MS = 30_000;

// The protocol's own headers - cookies and credentials stay behind, since the endpoint answers every agent the same
const FORWARDED = ["content-type", "accept", "mcp-protocol-version", "mcp-session-id", "last-event-id"];
const RETURNED = ["content-type", "allow", "mcp-session-id", "cache-control"];

// The storefront's /api/mcp - an agent always shops this store's channel, and the client address is only as good as the edge in front
export async function proxyMcp(req: Request, backend: string = FRONT_MCP_URL): Promise<Response> {
  let body: string | undefined;
  if (req.method === "POST") {
    const text = await boundedText(req, MAX_BODY_BYTES);
    if (text === null) return new Response("Request body too large", { status: 413 });
    body = text;
  }
  const headers = new Headers({ "X-Channel-Id": await request().channelId() });
  for (const h of FORWARDED) {
    const v = req.headers.get(h);
    if (v) headers.set(h, v);
  }
  const ip = clientIP(req.headers);
  if (ip) headers.set("X-Forwarded-For", ip);
  let res: Response;
  try {
    res = await fetch(backend, { method: req.method, headers, body, cache: "no-store", signal: AbortSignal.timeout(TIMEOUT_MS) });
  } catch {
    return new Response("Bad gateway", { status: 502 });
  }
  const out = new Headers();
  for (const h of RETURNED) {
    const v = res.headers.get(h);
    if (v) out.set(h, v);
  }
  return new Response(res.body, { status: res.status, headers: out });
}
