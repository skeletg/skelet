// Path-based filter + sort + page encoding for listing pages - grammar in ARCHITECTURE.md
// /<entity>/f/<attr>/v/<val>.../s/<sort>/pg/<page> - filters first, then sort, page last
// Import-safe from server and client - no React or Next imports

export type FilterChain = {
  filters: { slug: string; values: string[] }[];
};

export type SplitPath = {
  entityPath: string;
  filters: { slug: string; values: string[] }[];
  sortSlug: string;
  page: number;
};

const FILTER_MARKER = "f";
const VALUE_MARKER = "v";
const SORT_MARKER = "s";
// Exported so the catch-all can re-serialise a chain without re-hardcoding the marker
export const PAGE_MARKER = "pg";

const CHAIN_MARKERS = new Set([FILTER_MARKER, SORT_MARKER, PAGE_MARKER]);

export function isChainStart(seg: string): boolean {
  return CHAIN_MARKERS.has(seg);
}

export function splitSlug(slug: string[]): SplitPath {
  const decoded = slug.map((s) => decodeURIComponent(s));
  const chainStart = decoded.findIndex(isChainStart);
  if (chainStart === -1) {
    return { entityPath: decoded.join("/"), filters: [], sortSlug: "", page: 1 };
  }
  const chain = decoded.slice(chainStart);
  return {
    entityPath: decoded.slice(0, chainStart).join("/"),
    filters: parseFilterSegments(chain),
    sortSlug: parseSortFromSegments(chain),
    page: parsePageFromSegments(chain),
  };
}

// ["f","brand","v","atelier","v","cream"] -> [{slug:"brand",values:[...]}] Malformed
// input is dropped, not thrown - a typo'd URL renders unfiltered
export function parseFilterSegments(
  segments: string[],
): { slug: string; values: string[] }[] {
  const out: { slug: string; values: string[] }[] = [];
  let current: { slug: string; values: string[] } | null = null;
  for (let i = 0; i < segments.length; i++) {
    const seg = segments[i];
    if (seg === FILTER_MARKER) {
      const slug = segments[i + 1];
      if (!slug) break;
      current = { slug, values: [] };
      out.push(current);
      i++; // consume slug
    } else if (seg === VALUE_MARKER) {
      const value = segments[i + 1];
      if (!value || !current) {
        i++; // skip orphan value
        continue;
      }
      current.values.push(value);
      i++; // consume value
    } else if (seg === SORT_MARKER || seg === PAGE_MARKER) {
      // Same chain but not a filter - consume the payload so a later rework of this
      // loop can't mistake it for an attribute
      i++;
    }
    // Ignore unknown markers rather than letting them pollute the filter set
  }
  // Drop valueless attributes - they'd re-emit as /f/brand with no /v/, which
  // round-trips fine but means "no filter" to the backend
  return out.filter((f) => f.values.length > 0);
}

// First /s/<slug> wins - hand-edited duplicates collapse, same tolerant-on-read
// canonical-on-write rule the filter parser follows
export function parseSortFromSegments(segments: string[]): string {
  for (let i = 0; i < segments.length; i++) {
    if (segments[i] === SORT_MARKER) {
      const slug = segments[i + 1];
      if (slug) return slug;
      break;
    }
  }
  return "";
}

// First /pg/<n> wins, and anything that isn't a positive integer reads as page 1 - a
// typo'd page segment renders the first page rather than 404ing, same tolerant-on-read
// rule the filter and sort parsers follow
export function parsePageFromSegments(segments: string[]): number {
  for (let i = 0; i < segments.length; i++) {
    if (segments[i] === PAGE_MARKER) {
      const n = Number.parseInt(segments[i + 1] ?? "", 10);
      return Number.isFinite(n) && n >= 1 ? n : 1;
    }
  }
  return 1;
}

// Appends the page segment to an already-built chain path - page 1 stays the bare URL,
// so the first page never has a second address to canonicalise away
export function withPageSegment(path: string, page: number): string {
  if (page <= 1) return path;
  const trimmed = path.endsWith("/") ? path.slice(0, -1) : path;
  return `${trimmed}/${PAGE_MARKER}/${page}`;
}

// Chain -> URL. Alphabetical attributes and values, so two click paths to the same
// state can't produce two URLs for Google to index
export function buildFilterPath(
  filters: { slug: string; values: string[] }[],
): string {
  const sorted = [...filters]
    .filter((f) => f.values.length > 0)
    .sort((a, b) => a.slug.localeCompare(b.slug));
  const parts: string[] = [];
  for (const f of sorted) {
    parts.push(FILTER_MARKER, encodeURIComponent(f.slug));
    const values = [...f.values].sort();
    for (const v of values) {
      parts.push(VALUE_MARKER, encodeURIComponent(v));
    }
  }
  return parts.join("/");
}

export function toggleFilterValue(
  filters: { slug: string; values: string[] }[],
  attrSlug: string,
  bucketSlug: string,
): { slug: string; values: string[] }[] {
  const out = filters.map((f) => ({ slug: f.slug, values: [...f.values] }));
  const idx = out.findIndex((f) => f.slug === attrSlug);
  if (idx === -1) {
    out.push({ slug: attrSlug, values: [bucketSlug] });
    return out;
  }
  const existing = out[idx];
  if (existing.values.includes(bucketSlug)) {
    existing.values = existing.values.filter((v) => v !== bucketSlug);
  } else {
    existing.values.push(bucketSlug);
  }
  return out.filter((f) => f.values.length > 0);
}

export function joinBasePathWithFilters(
  basePath: string,
  filters: { slug: string; values: string[] }[],
  sortSlug = "",
  searchString = "",
): string {
  const trimmed = basePath.endsWith("/") ? basePath.slice(0, -1) : basePath;
  const filterChain = buildFilterPath(filters);
  const sortChain = sortSlug
    ? `${SORT_MARKER}/${encodeURIComponent(sortSlug)}`
    : "";
  const chain = [filterChain, sortChain].filter(Boolean).join("/");
  const path = chain ? `${trimmed}/${chain}` : trimmed || "/";
  return searchString ? `${path}?${searchString}` : path;
}

export type FacetH1Position = "BEFORE" | "AFTER";

export type FacetGroupForIndexability = {
  slug: string;
  h1Position: FacetH1Position;
  buckets: {
    value: string;
    slug: string;
    isIndexable: boolean;
    description: string;
  }[];
};

export type ResolvedFilterValue = {
  display: string;
  position: FacetH1Position;
  // Operator-set copy under the H1 - empty when not translated
  description: string;
};

export type SortOptionForIndexability = {
  slug: string;
  isIndexable: boolean;
};

export function isSortUrlIndexable(
  sortSlug: string,
  options: SortOptionForIndexability[],
): boolean {
  if (options.length === 0) return true; // backend soft-failed - don't punish the page.
  if (!sortSlug) return true; // no /s/ segment present.
  if (sortSlug === options[0].slug) return true; // explicit-default, still canonical.
  const match = options.find((o) => o.slug === sortSlug);
  if (!match) return false;
  return match.isIndexable;
}

// How deep each listing axis may be indexed, from seo/facets/max_indexable_page[_unfiltered].
// 0 on an axis lifts its cap
export type PageIndexCaps = { filtered: number; unfiltered: number };

// Is page N of a listing allowed into the index? Filters and pages are multiplicative - every
// indexable facet value times every page is a fresh near-duplicate URL - so the filtered axis
// carries the tighter cap (page 1 by default) and the plain pager is left uncapped, which is
// where deep products earn their way in
//
// Callers pair this with noindex,FOLLOW, never nofollow: a capped page still has to be walked
// for the products past it to be reachable at all under load-more paging
export function isPagedUrlIndexable(
  page: number,
  hasFilters: boolean,
  caps: PageIndexCaps,
): boolean {
  const cap = hasFilters ? caps.filtered : caps.unfiltered;
  if (cap <= 0) return true; // no cap on this axis
  return page <= cap;
}

export type FilterIndexability = {
  indexable: boolean;
  values: ResolvedFilterValue[];
};

export type FacetIndexRules = {
  // Sorted lowercased slug arrays - a multi-facet chain is indexable only when its
  // facet set matches one of these (seo/facets/indexable_combinations)
  combinations: string[][];
};

// Does the chain's facet-slug set match an allowed combination? Single facet is
// always structurally fine; multi-value picks of one facet are OR pages that only
// duplicate their single-value parents - never indexable
export function filterChainAllowed(
  chain: { slug: string; values: string[] }[],
  rules: FacetIndexRules,
): boolean {
  if (!chain.every((f) => f.values.length === 1)) return false;
  if (chain.length <= 1) return true;
  const key = chain.map((f) => f.slug.toLowerCase()).sort().join("+");
  return rules.combinations.some((combo) => combo.join("+") === key);
}

// Active chain vs the BrowseProducts facets - one missing or false bucket drops the
// whole page to noindex, and so does an unlisted facet combination (see
// filterChainAllowed): every extra crawlable combination multiplies the URL space
export function evaluateFilterIndexability(
  pathFilters: { slug: string; values: string[] }[],
  facets: FacetGroupForIndexability[],
  rules: FacetIndexRules = { combinations: [] },
): FilterIndexability {
  if (pathFilters.length === 0) {
    return { indexable: true, values: [] };
  }
  const byAttr = new Map(facets.map((f) => [f.slug, f] as const));
  const out: ResolvedFilterValue[] = [];
  let indexable = filterChainAllowed(pathFilters, rules);
  for (const f of pathFilters) {
    const group = byAttr.get(f.slug);
    const position: FacetH1Position = group?.h1Position ?? "BEFORE";
    for (const v of f.values) {
      const bucket = group?.buckets.find((b) => b.slug === v);
      if (!bucket || !bucket.isIndexable) {
        indexable = false;
      }
      out.push({
        display: bucket?.value ?? v,
        position,
        description: bucket?.description ?? "",
      });
    }
  }
  return { indexable, values: out };
}

export function assembleH1(
  baseName: string,
  result: FilterIndexability,
): string {
  if (!result.indexable || result.values.length === 0) {
    return baseName;
  }
  const before = result.values
    .filter((v) => v.position === "BEFORE")
    .map((v) => v.display);
  const after = result.values
    .filter((v) => v.position === "AFTER")
    .map((v) => v.display);
  return [...before, baseName, ...after].join(" ");
}
