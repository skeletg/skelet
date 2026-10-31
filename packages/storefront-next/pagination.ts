// PLP paging math, shared by the shell, the pager components, and generateMetadata
//
// The page lives in the PATH (/pg/<n>), the same chain the facets and sort use - so a
// paginated listing is a clean crawlable URL rather than a parameterised duplicate of
// its base. parsePage below reads only the plain ?page= form, which the shell
// redirects away

import { withPageSegment } from "./filter-path";

export type PlpSlicePlan =
  | { cumulative: true; offsets: number[] }
  | { cumulative: false; offset: number };

// Cumulative rendering fans out one slice query per page, so this bounds what a deep
// crawler URL can cost: past the cap ?page=N degrades to a classic windowed page
// (single query, full pager) - 15 pages at the default 24/page is 360 rendered items
export const CUMULATIVE_PAGE_CAP = 15;

// The 1-based page index out of a plain ?page= query param - live URLs carry the page
// as a path segment (parsePageFromSegments); this only exists so the shell can spot a
// hand-written or externally linked URL and redirect it to the path form
export function parsePage(sp: Record<string, string | string[] | undefined>): number {
  const raw = sp.page;
  const str = Array.isArray(raw) ? raw[0] : raw;
  if (!str) return 1;
  const n = Number.parseInt(str, 10);
  return Number.isFinite(n) && n >= 1 ? n : 1;
}

// basePath is the already-built filter+sort chain; the page rides on the end of it as
// /pg/<n>. Preserved params stay in the query string - they're things like the search
// term `q`, which is not part of the listing's path grammar
export function hrefForPage(
  basePath: string,
  preservedParams: Record<string, string>,
  page: number,
): string {
  const path = withPageSegment(basePath, page);
  const s = new URLSearchParams(preservedParams).toString();
  return s ? `${path}?${s}` : path;
}

export function planSlices(page: number, pageSize: number): PlpSlicePlan {
  if (page <= CUMULATIVE_PAGE_CAP) {
    const offsets: number[] = [];
    for (let p = 0; p < page; p++) offsets.push(p * pageSize);
    return { cumulative: true, offsets };
  }
  return { cumulative: false, offset: (page - 1) * pageSize };
}

// Concat slices in offset order, dropping repeats by id - stock or index changes between
// slice queries can shift an item across a boundary, and a duplicate would collide as a
// React key besides counting twice
export function mergeSlices<T extends { id: string }>(slices: T[][]): T[] {
  const seen = new Set<string>();
  const out: T[] = [];
  for (const slice of slices) {
    for (const item of slice) {
      if (seen.has(item.id)) continue;
      seen.add(item.id);
      out.push(item);
    }
  }
  return out;
}
