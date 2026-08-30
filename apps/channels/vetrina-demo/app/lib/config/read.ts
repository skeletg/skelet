import { cache } from "react";

import {
  PublicConfigDocument,
  type PublicConfigQuery,
  StoreFeaturesDocument,
  type StoreFeaturesQuery,
} from "@/app/lib/config/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";
import { getCurrentLocale, getCurrentChannelId } from "@/app/lib/locale/read";

const BROWSE_PAGE_SIZE_PATH = "search/browse/page_size_default";
const DEFAULT_BROWSE_PAGE_SIZE = 24;

const PLP_PAGINATION_STYLE_PATH = "search/browse/pagination_style";

const ALLOW_GUEST_PATH = "sales/checkout/allow_guest";

export type PlpPaginationStyle = "pagination" | "load_more" | "hybrid";

// getPlpPaginationStyle picks the PLP pager UI - anything absent, unknown, or unreachable falls
// back to classic numbered pagination, the pre-setting behaviour
export const getPlpPaginationStyle = cache(async (): Promise<PlpPaginationStyle> => {
  try {
    const res = await gqlFetch(PublicConfigDocument, {});
    const data = res.data as PublicConfigQuery;
    const row = data.publicConfig.find((c) => c.path === PLP_PAGINATION_STYLE_PATH);
    return row?.value === "load_more" || row?.value === "hybrid" ? row.value : "pagination";
  } catch {
    return "pagination";
  }
});

// Whether checkout accepts an order from someone who isn't signed in. The backend enforces this at
// placement (GUEST_CHECKOUT_DISABLED); reading it here just means the shopper is told before they
// fill the form rather than after. Absent or unreachable reads as allowed, matching the default
export const getGuestCheckoutAllowed = cache(async (): Promise<boolean> => {
  try {
    const res = await gqlFetch(PublicConfigDocument, {});
    const data = res.data as PublicConfigQuery;
    const row = data.publicConfig.find((c) => c.path === ALLOW_GUEST_PATH);
    return row?.value !== "false";
  } catch {
    return true;
  }
});

// getBrowsePageSize returns the configured PLP page size, falling back to DEFAULT_BROWSE_PAGE_SIZE when the value is
// absent, unparseable, or the backend is unreachable - the grid must still render
export const getBrowsePageSize = cache(async (): Promise<number> => {
  try {
    const res = await gqlFetch(PublicConfigDocument, {});
    // res.data is `unknown` here - the generated typed-document-node generics don't
    // resolve under this tsconfig (see any *.gql.ts)
    const data = res.data as PublicConfigQuery;
    const row = data.publicConfig.find((c) => c.path === BROWSE_PAGE_SIZE_PATH);
    const n = row ? Number.parseInt(row.value, 10) : Number.NaN;
    return Number.isFinite(n) && n > 0 ? n : DEFAULT_BROWSE_PAGE_SIZE;
  } catch {
    return DEFAULT_BROWSE_PAGE_SIZE;
  }
});

// general/urls/variant_own_url - on, a variant gets its own URL and preselects on the parent PDP; off, variants behave
// as plain simples cache()-wrapped so the PDP's metadata and body share one publicConfig fetch
const VARIANT_OWN_URL_PATH = "general/urls/variant_own_url";

export const getVariantOwnUrl = cache(async (): Promise<boolean> => {
  try {
    const res = await gqlFetch(PublicConfigDocument, {}, {
      cache: "force-cache",
      revalidate: 300,
      tags: [CONFIG_TAG],
      // publicConfig resolves per channel and Next keys the Data Cache off the request body, not
      // the X-Channel-Id header - without this one channel's values would be served to all
      cacheKeyExtra: [await getCurrentChannelId()],
    });
    const data = res.data as PublicConfigQuery;
    const row = data.publicConfig.find((c) => c.path === VARIANT_OWN_URL_PATH);
    // Absent / unparseable / backend down → default ON (matches the backend
    // default), so the feature degrades gracefully rather than silently off
    return row ? row.value !== "false" : true;
  } catch {
    return true;
  }
});

// general/branding/store_name - the CUSTOMER-facing store name. channels.name is the operator's
// internal label ("US Wholesale B2B"), which is what shoppers saw before this. Empty means unset;
// resolving the fallback needs the channel, so that lives in lib/channel/read.ts to keep the
// import direction one-way
const STORE_NAME_PATH = "general/branding/store_name";

export const getStoreNameSetting = cache(async (): Promise<string | null> => {
  try {
    const res = await gqlFetch(PublicConfigDocument, {}, {
      cache: "force-cache",
      revalidate: 300,
      tags: [CONFIG_TAG],
      // Per-channel, and Next keys the Data Cache off the request body rather than X-Channel-Id
      cacheKeyExtra: [await getCurrentChannelId()],
    });
    const data = res.data as PublicConfigQuery;
    const row = data.publicConfig.find((c) => c.path === STORE_NAME_PATH);
    const trimmed = row?.value.trim() ?? "";
    return trimmed === "" ? null : trimmed;
  } catch {
    return null;
  }
});

// general/branding/{logo_url,favicon_url} - both TypeImage, so the stored value is an absolute URL
// into media storage. One fetch for the pair: publicConfig is a single document and cache()d, so
// asking for them separately would still be one request, but this keeps the call sites honest about
// reading a branding set rather than two unrelated knobs
const LOGO_PATH = "general/branding/logo_url";
const FAVICON_PATH = "general/branding/favicon_url";
const PRIMARY_COLOR_PATH = "general/branding/primary_color";
const SECONDARY_COLOR_PATH = "general/branding/secondary_color";

export type Branding = {
  logoUrl: string | null;
  faviconUrl: string | null;
  primaryColor: string | null;
  secondaryColor: string | null;
};

// The backend already stores TypeColor as a validated #rrggbb, but these values end up inside a
// <style> block, so they are re-checked here rather than trusted: a row written straight to the
// database, or predating that validation, would otherwise be CSS injection
const HEX_COLOR = /^#[0-9a-f]{6}$/i;

export const getBranding = cache(async (): Promise<Branding> => {
  try {
    const res = await gqlFetch(PublicConfigDocument, {}, {
      cache: "force-cache",
      revalidate: 300,
      tags: [CONFIG_TAG],
      cacheKeyExtra: [await getCurrentChannelId()],
    });
    const data = res.data as PublicConfigQuery;
    const pick = (path: string): string | null => {
      const v = data.publicConfig.find((c) => c.path === path)?.value.trim() ?? "";
      return v === "" ? null : v;
    };
    const color = (path: string): string | null => {
      const v = pick(path);
      return v && HEX_COLOR.test(v) ? v : null;
    };
    return {
      logoUrl: pick(LOGO_PATH),
      faviconUrl: pick(FAVICON_PATH),
      primaryColor: color(PRIMARY_COLOR_PATH),
      secondaryColor: color(SECONDARY_COLOR_PATH),
    };
  } catch {
    return { logoUrl: null, faviconUrl: null, primaryColor: null, secondaryColor: null };
  }
});

// Server-wide feature switches (env-driven on the backend, e.g. B2B_ENABLED /
// SIGNALS_ENABLED)
const getFeatures = cache(async (): Promise<{ b2b: boolean; signals: boolean }> => {
  try {
    const res = await gqlFetch(StoreFeaturesDocument, {}, {
      cache: "force-cache",
      revalidate: 300,
      tags: [CONFIG_TAG],
    });
    const data = res.data as StoreFeaturesQuery;
    return data.features;
  } catch {
    return { b2b: true, signals: true };
  }
});

export async function getB2BEnabled(): Promise<boolean> {
  return (await getFeatures()).b2b;
}

export async function getSignalsEnabled(): Promise<boolean> {
  return (await getFeatures()).signals;
}

// Per-channel SEO config driving the sitemap + robots.txt routes
export type SeoConfig = {
  products: boolean;
  categories: boolean;
  collections: boolean;
  cms: boolean;
  blog: boolean;
  excludePaths: string[];
  extraUrls: string[];
  robotsOverride: string;
  // Clamped to the sitemap spec's 50k-per-file ceiling
  maxUrlsPerFile: number;
  // Multi-facet filter pages are indexable only when their facet-slug set matches one
  // of these lines (sorted slug arrays); single-facet pages index per value flags
  facetCombinations: string[][];
  // Filter pages with fewer results go noindex (0 = off)
  minFacetResults: number;
  // What the robots meta says once indexability is decided - undefined means no tag
  robotsNoindex: { index: false; follow: boolean };
  robotsIndexable: { index: true; follow: true } | undefined;
  // How deep each listing axis may be indexed before pages go noindex - filters and pages
  // multiply, so capping the filtered axis is what keeps them from compounding into a crawl
  // trap. 0 means no cap on that axis
  pageIndexCaps: PageIndexCaps;
};

export type PageIndexCaps = { filtered: number; unfiltered: number };

const SEO_DEFAULTS: SeoConfig = {
  products: true,
  categories: true,
  collections: true,
  cms: true,
  blog: true,
  excludePaths: [],
  extraUrls: [],
  robotsOverride: "",
  maxUrlsPerFile: 45000,
  facetCombinations: [],
  minFacetResults: 0,
  robotsNoindex: { index: false, follow: true },
  robotsIndexable: undefined,
  pageIndexCaps: { filtered: 1, unfiltered: 0 },
};

// Reads the SEO settings in one publicConfig fetch - every field falls back to its registered default, so a missing key
// or an unreachable backend still yields a valid sitemap and robots.txt rather than an error
export const getSeoConfig = cache(async (): Promise<SeoConfig> => {
  try {
    const res = await gqlFetch(PublicConfigDocument, {}, {
      cache: "force-cache",
      revalidate: 3600,
      tags: [CONFIG_TAG],
      cacheKeyExtra: [await getCurrentChannelId()],
    });
    const data = res.data as PublicConfigQuery;
    const val = (path: string) => data.publicConfig.find((c) => c.path === path)?.value;
    // absent → the BOOL default is true, so a new/old backend both index everything
    const bool = (path: string) => val(path) !== "false";
    const lines = (path: string) =>
      (val(path) ?? "").split("\n").map((s) => s.trim()).filter(Boolean);
    // A page cap: 0 (and anything unparseable or negative) means no cap on that axis
    const pageCap = (path: string, fallback: number) => {
      const raw = val(path);
      if (raw === undefined) return fallback;
      const n = Number.parseInt(raw, 10);
      return Number.isFinite(n) && n > 0 ? n : 0;
    };
    return {
      products: bool("seo/sitemap/include_products"),
      categories: bool("seo/sitemap/include_categories"),
      collections: bool("seo/sitemap/include_collections"),
      cms: bool("seo/sitemap/include_cms"),
      blog: bool("seo/sitemap/include_blog"),
      excludePaths: lines("seo/sitemap/exclude_paths"),
      extraUrls: lines("seo/sitemap/extra_urls"),
      robotsOverride: (val("seo/robots/robots_txt") ?? "").trim(),
      maxUrlsPerFile: Math.min(
        50000,
        Math.max(1, Number(val("seo/sitemap/max_urls_per_file")) || SEO_DEFAULTS.maxUrlsPerFile),
      ),
      facetCombinations: lines("seo/facets/indexable_combinations")
        .map((line) =>
          line
            .split("+")
            .map((slug) => slug.trim().toLowerCase())
            .filter(Boolean)
            .sort(),
        )
        .filter((combo) => combo.length >= 2),
      minFacetResults: (() => {
        const raw = Number(val("seo/facets/min_results"));
        return Number.isFinite(raw) && raw > 0 ? raw : 0;
      })(),
      robotsNoindex: {
        index: false,
        follow: val("seo/facets/robots_noindex") !== "noindex,nofollow",
      },
      robotsIndexable:
        val("seo/facets/robots_indexable") === "index,follow"
          ? { index: true, follow: true }
          : undefined,
      pageIndexCaps: {
        filtered: pageCap("seo/facets/max_indexable_page", SEO_DEFAULTS.pageIndexCaps.filtered),
        unfiltered: pageCap(
          "seo/facets/max_indexable_page_unfiltered",
          SEO_DEFAULTS.pageIndexCaps.unfiltered,
        ),
      },
    };
  } catch {
    return SEO_DEFAULTS;
  }
});

// The cache-related subset of gqlFetch's options
export type CacheFetchOpts = {
  cache?: "force-cache";
  revalidate?: number;
  tags?: string[];
  cacheKeyExtra?: string[];
};

// Data Cache lifetime per surface. Only a backstop: on a full-page-cached route the page's
// own statically-analysable `revalidate` governs the HTML, and a fetch TTL below it would
// drag the whole route's regeneration down with it. This matters on routes that still
// render per request (checkout, account, blog)
const SURFACE_TTL_SECONDS: Record<string, number> = {
  nav: 3600,
  cms: 3600,
  product: 3600,
  catalog: 3600,
};
const FALLBACK_TTL_SECONDS = 3600;

// The one decision point for storefront data caching, and the dev/prod switch:
// dev always returns {} so every fetch is no-store and you never chase a stale cache
//
// Tags are attached unconditionally on purpose - gating them on the admin's cache/<surface>/enabled
// cannot work: `revalidate` has to be statically analysable, so disabling a surface would strip the
// tags without stopping the HTML caching, leaving pages cached and UNPURGEABLE
//
// token makes a surface anonymous-only - a personalized response must never be shared
// keyExtra overrides the default [store, locale] key dimensions
export async function surfaceCacheOpts(
  key: string,
  opts: { tags: string[]; token?: string | null; keyExtra?: string[] },
): Promise<CacheFetchOpts> {
  // Developer mode: never cache
  if (process.env.NODE_ENV !== "production") return {};
  // Never share a signed-in visitor's (potentially personalized) response
  if (opts.token) return {};
  const keyExtra =
    opts.keyExtra ?? [await getCurrentChannelId(), await getCurrentLocale()];
  return {
    cache: "force-cache",
    revalidate: SURFACE_TTL_SECONDS[key] ?? FALLBACK_TTL_SECONDS,
    tags: opts.tags,
    cacheKeyExtra: keyExtra,
  };
}

// Well-known "nav" cache surface: the store-scoped, rarely-changing data the root layout + header pull on every page
// (channel, menus, presentment, free-shipping threshold)
export const CONFIG_TAG = "config:public";
export const NAV_SURFACE = "nav";
export const NAV_TAG = "nav:all";
