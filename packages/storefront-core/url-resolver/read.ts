// Server-side helper that asks the backend "what entity lives at this path?"


import { perRequest } from "../request";
import { gqlFetch } from "../graphql";
import { surfaceCacheOpts } from "../config/read";
import { CATALOG_SURFACE } from "../catalog/read";
import { RouteDocument } from "./operations.gql";
import type { RouteKind } from "../graphql/schema.types";

export type ResolvedRoute = {
  kind: RouteKind;
  entityId: string;
  redirectTo: string | null;
  redirectStatusCode: number | null;
};

// React.cache so a render path that resolves the same URL twice in one request (e.g.
// generateMetadata + the page body) only roundtrips once
export const resolveRoute = perRequest(async (path: string): Promise<ResolvedRoute | null> => {
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
