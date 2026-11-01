// Catalog sitemap data layer


import { perRequest } from "../request";
import { getSeoConfig } from "../config/read";
import { gqlFetch } from "../graphql";
import { SitemapCountsDocument, SitemapEntriesDocument } from "./operations.gql";
import { SITE_URL, absUrl } from "../site-url";
import type { SitemapEntityType } from "../graphql/schema.types";

// Fallback only - the live size comes from the seo/sitemap/max_urls_per_file setting
export const SITEMAP_CHUNK = 45000;

export { absUrl };
export { getSiteUrl } from "../channel/site";

// Boundary-aware: excluding "/blog" drops "/blog" and "/blog/x" but not "/blogging-tips"
function matchesExclude(path: string, rule: string): boolean {
  return path === rule || path.startsWith(rule.endsWith("/") ? rule : rule + "/");
}

// The one place every sitemap route applies the operator's exclude_paths setting -
// fetches the config itself so no caller can forget half the rule
export async function applySitemapExclusions<T extends { url: string }>(
  entries: T[],
  base: string = SITE_URL,
): Promise<T[]> {
  const { excludePaths } = await getSeoConfig();
  if (excludePaths.length === 0) return entries;
  return entries.filter((e) => {
    const path = e.url.startsWith(base) ? e.url.slice(base.length) || "/" : e.url;
    return !excludePaths.some((rule) => matchesExclude(path, rule));
  });
}

export type SitemapCounts = { products: number; categories: number; collections: number };

// Zeroes on failure so sitemap routes degrade to valid empty files instead of 500ing -
// crawlers treat persistent 5xx on sitemaps as a bad signal
export const getSitemapCounts = perRequest(async (): Promise<SitemapCounts> => {
  try {
    const res = await gqlFetch(SitemapCountsDocument, {});
    return res.data.sitemapCounts;
  } catch {
    return { products: 0, categories: 0, collections: 0 };
  }
});

export async function listSitemapEntries(
  entityType: SitemapEntityType,
  chunk: number,
  chunkSize: number = SITEMAP_CHUNK,
): Promise<{ path: string; updatedAt: string }[]> {
  try {
    const res = await gqlFetch(SitemapEntriesDocument, {
      entityType,
      limit: chunkSize,
      offset: chunk * chunkSize,
    });
    return res.data.sitemapEntries;
  } catch {
    return [];
  }
}

// Always at least one chunk so /…/sitemap/0.xml exists (a valid empty sitemap) even
// before the catalog has content
export function chunkCount(total: number, chunkSize: number = SITEMAP_CHUNK): number {
  return Math.max(1, Math.ceil(total / chunkSize));
}
