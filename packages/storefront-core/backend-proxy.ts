import { BACKEND_INTERNAL_URL } from "./backend-url";

// The backend's same-origin links: thumbnails, downloads, unsubscribe and signup confirmation, email pixels and clicks, and the /t beacons
export const PROXIED_PREFIXES = ["/media/t/", "/download/", "/unsubscribe/", "/subscribe/", "/e/", "/t/"] as const;

const HOP_BY_HOP = [
  "connection",
  "keep-alive",
  "proxy-connection",
  "transfer-encoding",
  "te",
  "trailer",
  "upgrade",
  "host",
  "content-length",
];

export async function proxyToBackend(req: Request, backend: string = BACKEND_INTERNAL_URL): Promise<Response> {
  const src = new URL(req.url);
  // As parsed - /t/%2e%2e/api/admin matches the /t/ route, then resolves outside it
  if (!PROXIED_PREFIXES.some((p) => src.pathname.startsWith(p))) return new Response("Not found", { status: 404 });
  const headers = new Headers(req.headers);
  for (const h of HOP_BY_HOP) headers.delete(h);
  headers.set("accept-encoding", "identity");
  const hasBody = req.method !== "GET" && req.method !== "HEAD";
  let res: Response;
  try {
    res = await fetch(backend.replace(/\/+$/, "") + src.pathname + src.search, {
      method: req.method,
      headers,
      body: hasBody ? await req.arrayBuffer() : undefined,
      redirect: "manual",
      cache: "no-store",
    });
  } catch {
    return new Response("Bad gateway", { status: 502 });
  }
  const out = new Headers(res.headers);
  for (const h of HOP_BY_HOP) out.delete(h);
  if (out.has("content-encoding")) out.delete("content-encoding");
  else if (res.headers.has("content-length")) out.set("content-length", res.headers.get("content-length")!);
  return new Response(req.method === "HEAD" ? null : res.body, {
    status: res.status,
    statusText: res.statusText,
    headers: out,
  });
}

export const backendRoute = (req: Request) => proxyToBackend(req);
