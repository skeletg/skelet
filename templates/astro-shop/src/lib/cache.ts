import type { APIContext } from "astro";

import { CONFIG_TAG, NAV_TAG, surfaceTTL } from "@skelet/storefront-core/config/read";

// only outside a server (the Container API in tests)
export async function cacheCatalog(cache: APIContext["cache"] | undefined, surface: string, tags: string[]): Promise<void> {
  if (!cache) return;
  const maxAge = await surfaceTTL(surface);
  cache.set({ maxAge, swr: Math.min(600, maxAge), tags: [...tags, NAV_TAG, CONFIG_TAG] });
}
