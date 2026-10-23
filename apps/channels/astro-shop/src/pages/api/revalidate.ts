import type { APIRoute } from "astro";
import { timingSafeEqual } from "node:crypto";

import { forgetChannel } from "../../lib/channel";

// The backend POSTs {tags, paths, warm} here when catalog or config changes (internal/revalidate) - the same
// contract vetrina-demo's route implements. tags/paths evict from the route cache, warm re-renders right away

function secretOk(provided: string | null): boolean {
  const expected = process.env.STOREFRONT_REVALIDATE_SECRET ?? "";
  if (!expected || !provided) return false;
  const a = Buffer.from(provided);
  const b = Buffer.from(expected);
  return a.length === b.length && timingSafeEqual(a, b);
}

function strings(v: unknown, max: number): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string" && x.length > 0).slice(0, max) : [];
}

const slash = (p: string) => (p.startsWith("/") ? p : `/${p}`);

export const POST: APIRoute = async ({ request, cache, url }) => {
  if (!secretOk(request.headers.get("x-revalidate-secret"))) return Response.json({ error: "unauthorized" }, { status: 401 });
  let payload: { tags?: unknown; paths?: unknown; warm?: unknown };
  try {
    payload = await request.json();
  } catch {
    return Response.json({ error: "invalid json" }, { status: 400 });
  }
  const tags = strings(payload.tags, 64);
  const paths = strings(payload.paths, 64).map(slash);
  const warm = strings(payload.warm, 25).map(slash);

  if (tags.length > 0) await cache.invalidate({ tags });
  for (const path of paths) await cache.invalidate({ path });
  if (tags.includes("config:public")) forgetChannel();

  // Each warm is its own request, so it renders after the eviction above and lands back in the cache
  const results = await Promise.allSettled(
    warm.map((p) => fetch(new URL(p, url.origin), { signal: AbortSignal.any([request.signal, AbortSignal.timeout(15_000)]) })),
  );
  const warmed = results.filter((r) => r.status === "fulfilled" && r.value.ok).length;
  return Response.json({ revalidated: true, tags: tags.length, paths: paths.length, warmed, failed: warm.length - warmed });
};
