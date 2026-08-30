// Catalog sitemap data layer

import { cache } from "react";

import { getSeoConfig } from "@/app/lib/config/read";
import { gqlFetch } from "@/app/lib/graphql";
import { SitemapCountsDocument, SitemapEntriesDocument } from "@/app/lib/sitemap/operations.gql";
import { SITE_URL, absUrl } from "@/app/lib/site-url";
import type { SitemapEntityType } from "@/app/graphql/schema.types";

// Fallback only - the live size comes from the seo/sitemap/max_urls_per_file setting
export const SITEMAP_CHUNK = 45000;

export { absUrl };

// Boundary-aware: excluding "/blog" drops "/blog" and "/blog/x" but not "/blogging-tips"
function matchesExclude(path: string, rule: string): boolean {
  return path === rule || path.startsWith(rule.endsWith("/") ? rule : rule + "/");
}

// The one place every sitemap route applies the operator's exclude_paths setting -
// fetches the config itself so no caller can forget half the rule
export async function applySitemapExclusions<T extends { url: string }>(
  entries: T[],
): Promise<T[]> {
  const { excludePaths } = await getSeoConfig();
  if (excludePaths.length === 0) return entries;
  return entries.filter((e) => {
    const path = e.url.startsWith(SITE_URL) ? e.url.slice(SITE_URL.length) || "/" : e.url;
    return !excludePaths.some((rule) => matchesExclude(path, rule));
  });
}

export type SitemapCounts = { products: number; categories: number; collections: number };

// Zeroes on failure so sitemap routes degrade to valid empty files instead of 500ing -
// crawlers treat persistent 5xx on sitemaps as a bad signal
export const getSitemapCounts = cache(async (): Promise<SitemapCounts> => {
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
