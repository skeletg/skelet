// Server-side helper that asks the backend "what entity lives at this path?"

import { cache } from "react";

import { gqlFetch } from "@/app/lib/graphql";
import { surfaceCacheOpts } from "@/app/lib/config/read";
import { CATALOG_SURFACE } from "@/app/lib/catalog/read";
import { RouteDocument } from "@/app/lib/url-resolver/operations.gql";
import type { RouteKind } from "@/app/graphql/schema.types";

export type ResolvedRoute = {
  kind: RouteKind;
  entityId: string;
  redirectTo: string | null;
  redirectStatusCode: number | null;
};

// React.cache so a render path that resolves the same URL twice in one request (e.g.
// generateMetadata + the page body) only roundtrips once
export const resolveRoute = cache(async (path: string): Promise<ResolvedRoute | null> => {
  const trimmed = path.trim();
  if (!trimmed) return null;
  try {
    const cacheOpts = await surfaceCacheOpts(CATALOG_SURFACE, {
      tags: ["catalog:all", "cms:all", "blog:all"], // a rewrite can point at any of them
    });
    const res = await gqlFetch(RouteDocument, { path: trimmed }, cacheOpts);
    return res.data.route ?? null;
  } catch {
    // Soft fail: the catch-all renders 404 on null
    return null;
  }
});
