import type { APIContext } from "astro";

// Catalog pages' one cache policy. Every one also renders the channel's public config, so config:public is always on.
// cache is absent only outside a server (the Container API in tests)
export function cacheCatalog(cache: APIContext["cache"] | undefined, tags: string[]): void {
  cache?.set({ maxAge: 3600, swr: 600, tags: [...tags, "config:public"] });
}
