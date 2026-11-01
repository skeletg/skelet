import type { APIRoute } from "astro";

import { invalidateData } from "@skelet/storefront-astro/data-cache";
import { readRevalidate, warmURL, warmedRedirect } from "@skelet/storefront-core/revalidate-payload";

// The backend POSTs {tags, paths, warm} here when catalog or config changes (internal/revalidate) - the same
// contract nextjs-vetrina's route implements. tags/paths evict from the route cache, warm re-renders right away

const WARM_TIMEOUT_MS = 15_000;

export const POST: APIRoute = async ({ request, cache, url }) => {
  const payload = await readRevalidate(request);
  if (payload instanceof Response) return payload;
  const { tags, paths, warm } = payload;

  if (tags.length > 0) await cache.invalidate({ tags });
  for (const path of paths) await cache.invalidate({ path });
  // The core's cached reads - the channel and its config - under the same tags
  if (tags.length > 0) invalidateData(tags);

  // Each warm is its own request, so it renders after the eviction above and lands back in the cache
  const results = await Promise.allSettled(
    warm.map(async (p) => {
      const target = warmURL(url.origin, p);
      if (!target) return false;
      const res = await fetch(target, { redirect: "manual", signal: AbortSignal.any([request.signal, AbortSignal.timeout(WARM_TIMEOUT_MS)]) });
      return res.ok || warmedRedirect(res);
    }),
  );
  const warmed = results.filter((r) => r.status === "fulfilled" && r.value).length;
  return Response.json({ revalidated: true, tags: tags.length, paths: paths.length, warmed, failed: warm.length - warmed });
};
