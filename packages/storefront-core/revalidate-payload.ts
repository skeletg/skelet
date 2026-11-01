import { isChannelId } from "./channel";
import { secretEquals } from "./secret-equal";

export const REVALIDATE_SECRET_HEADER = "x-revalidate-secret";

const MAX_BODY_BYTES = 1 << 20;
const MAX_WARM = 25;

export type RevalidatePayload = { tags: string[]; paths: string[]; warm: string[]; channels: string[] };

export async function readRevalidate(req: Request): Promise<RevalidatePayload | Response> {
  if (!secretEquals(req.headers.get(REVALIDATE_SECRET_HEADER), process.env.STOREFRONT_REVALIDATE_SECRET ?? "")) {
    return Response.json({ error: "unauthorized" }, { status: 401 });
  }
  const text = await boundedText(req, MAX_BODY_BYTES);
  if (text === null) return Response.json({ error: "payload too large" }, { status: 413 });
  let p: { tags?: unknown; paths?: unknown; warm?: unknown; channels?: unknown };
  try {
    p = JSON.parse(text);
  } catch {
    return Response.json({ error: "invalid json" }, { status: 400 });
  }
  return {
    tags: strings(p?.tags),
    paths: strings(p?.paths).map(slash),
    warm: strings(p?.warm).slice(0, MAX_WARM).map(slash),
    channels: strings(p?.channels).filter(isChannelId),
  };
}

export function warmURL(origin: string, path: string): string | null {
  try {
    const url = new URL(origin + slash(path));
    return url.origin === new URL(origin).origin ? url.href : null;
  } catch {
    return null;
  }
}

// A warm carries the revalidate secret, so a redirect it gets is the end of it, never followed
export function warmedRedirect(res: Response): boolean {
  return res.type === "opaqueredirect" || (res.status >= 300 && res.status < 400);
}

function strings(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string" && x.length > 0) : [];
}

function slash(p: string): string {
  return p.startsWith("/") ? p : `/${p}`;
}

async function boundedText(req: Request, max: number): Promise<string | null> {
  if (Number(req.headers.get("content-length") ?? 0) > max) return null;
  if (!req.body) return "";
  const reader = req.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > max) {
      await reader.cancel();
      return null;
    }
    chunks.push(value);
  }
  return new TextDecoder().decode(Buffer.concat(chunks));
}
