import { revalidatePath, revalidateTag } from "next/cache";
import { timingSafeEqual } from "node:crypto";

// Backend POSTs here on CMS publish/unpublish to bust Data Cache tags, and for on-demand rebuilds. tags/paths invalidate lazily; warm
// re-GETs each path now Auth is a constant-time compared X-Revalidate-Secret - never cache this handler
export const dynamic = "force-dynamic";

function secretOk(provided: string | null): boolean {
  const expected = process.env.STOREFRONT_REVALIDATE_SECRET ?? "";
  if (!expected || !provided) return false;
  const a = Buffer.from(provided);
  const b = Buffer.from(expected);
  if (a.length !== b.length) return false;
  return timingSafeEqual(a, b);
}

function stringList(v: unknown, max: number): string[] {
  if (!Array.isArray(v)) return [];
  return v.filter((x): x is string => typeof x === "string" && x.length > 0).slice(0, max);
}

export async function POST(req: Request) {
  if (!secretOk(req.headers.get("x-revalidate-secret"))) {
    return Response.json({ error: "unauthorized" }, { status: 401 });
  }
  let payload: { tags?: unknown; paths?: unknown; warm?: unknown };
  try {
    payload = (await req.json()) as { tags?: unknown; paths?: unknown; warm?: unknown };
  } catch {
    return Response.json({ error: "invalid json" }, { status: 400 });
  }
  const tags = stringList(payload.tags, 64);
  const paths = stringList(payload.paths, 64);
  const warm = stringList(payload.warm, 25);
  // { expire: 0 } = invalidate immediately (the on-demand/webhook form; the bare
  // single-arg revalidateTag is deprecated in this Next version)
  for (const tag of tags) revalidateTag(tag, { expire: 0 });
  for (const path of paths) revalidatePath(path);

  // Rebuild on demand: now that the relevant entries are stale, GET each warm path on
  // this origin so the page re-renders and re-populates the cache immediately
  let warmed = 0;
  if (warm.length > 0) {
    const origin = new URL(req.url).origin;
    const results = await Promise.allSettled(
      warm.map((p) => {
        const ctrl = AbortSignal.timeout(15_000);
        return fetch(origin + (p.startsWith("/") ? p : `/${p}`), {
          cache: "no-store",
          signal: ctrl,
        });
      }),
    );
    warmed = results.filter((r) => r.status === "fulfilled").length;
  }
  return Response.json({ revalidated: true, tags: tags.length, paths: paths.length, warmed });
}
