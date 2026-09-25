// Shared chrome for every list page (/products, /search, categories, collections) so
// they stay identical. Caller picks the base filter and forwards searchParams; facet
// selections layer on top and each checkbox is a Link, so the URL drives re-renders
// Page-specific chrome goes in the header/prefix slots, not in here

import { getSiteUrl } from "@/app/lib/channel/site";
import type { ReactNode } from "react";
import { permanentRedirect } from "next/navigation";
import { getLocale } from "next-intl/server";

import { ViewTransition } from "react";

import { FacetSidebar, type FacetGroupInput } from "./FacetSidebar";
import { Pagination } from "./Pagination";
import { ProductCard } from "./ProductCard";
import { SortSelect } from "./SortSelect";
import {
  browseProducts,
  browseProductsSlim,
  productSortOptions,
  synapseTrackedSlugs,
  type BrowseProductsItem,
  type ProductSummary,
} from "@/app/lib/catalog/read";
import { LoadMore } from "./LoadMore";
import { getSeoConfig } from "@/app/lib/config/read";
import { getBrowsePageSize, getPlpPaginationStyle } from "@/app/lib/config/read";
import type { BrowseProductsQueryVariables } from "@/app/lib/catalog/operations.gql";
import { joinBasePathWithFilters } from "@/app/lib/filter-path";
import { hrefForPage, mergeSlices, parsePage, planSlices } from "@/app/lib/pagination";
import { withLocalePrefix } from "@/app/lib/locale/path";
import { canonicalPathMapper } from "@/app/lib/locale/alternates";
import { productHref } from "@/app/lib/entity-href";
import { itemListJsonLd } from "@/app/lib/json-ld";
import { JsonLd } from "./JsonLd";
import { LensTrack } from "./LensTrack";

type RawSearchParams = Record<string, string | string[] | undefined>;

function describeFilters(
  attributes: { slug: string; values: string[] }[],
  facets: { slug: string; name: string; buckets: { slug: string; value: string }[] }[],
): string {
  if (attributes.length === 0) return "";
  const bySlug = new Map(facets.map((f) => [f.slug, f]));
  return attributes
    .map((a) => {
      const f = bySlug.get(a.slug);
      const label = f?.name ?? a.slug;
      const vals = a.values.map((vs) => f?.buckets.find((b) => b.slug === vs)?.value ?? vs);
      return `${label}: ${vals.join(", ")}`;
    })
    .join(" · ");
}

// Query params that are NOT attribute filters
const RESERVED_QUERY_KEYS = new Set(["q", "page"]);

function parseAttributeFilters(
  sp: RawSearchParams,
): { slug: string; values: string[] }[] {
  const out: { slug: string; values: string[] }[] = [];
  for (const [k, raw] of Object.entries(sp)) {
    if (raw === undefined || RESERVED_QUERY_KEYS.has(k)) continue;
    const joined = Array.isArray(raw) ? raw.join(",") : raw;
    const values = joined
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);
    if (values.length > 0) out.push({ slug: k, values });
  }
  return out;
}

type Props = {
  header?: ReactNode;
  prefix?: ReactNode;
  // Rendered full-width below the sidebar+grid row - the SEO copy block on category/collection pages
  suffix?: ReactNode;
  filter?: BrowseProductsQueryVariables["filter"];
  query?: string;
  // The page's searchParams from Next.js (already awaited)
  searchParams: RawSearchParams;
  // Pages that use path-based filters (category / collection / search) pre-parse the
  // chain and pass it in. When omitted, the shell falls back to query-string filtering
  // - that's the plain path /products and the /[...slug] catch-all still take
  pathFilters?: { slug: string; values: string[] }[];
  pathSort?: string;
  // The /pg/<n> segment, already parsed by the page - the page is part of the path
  // grammar, not a query param
  pathPage?: number;
  basePath?: string;
  limit?: number;
  emptyText?: string;
};

export async function ProductListShell({
  header,
  prefix,
  suffix,
  filter,
  query,
  searchParams,
  pathFilters,
  pathSort,
  pathPage,
  basePath,
  limit,
  emptyText = "No products match the current filters.",
}: Props) {
  // Page size: caller override wins; otherwise the admin-configured value
  const pageSize = limit ?? (await getBrowsePageSize());
  // Path-based callers feed pathFilters directly; query-based callers (/products today, the [...slug] catch-all when no
  // /f/ marker is present) fall back to parseAttributeFilters on searchParams
  const attributes = pathFilters ?? parseAttributeFilters(searchParams);

  // Sort is encoded as the path segment `/s/<slug>` and parsed by the catch-all pages - read it from the prop, not the
  // query string, so the URL stays canonical with no querystring noise
  const sortSlug = pathSort ?? "";

  const currentPage = pathPage ?? 1;

  // Pager href base: the entity path plus this listing's /f/../v/../s/.. chain - the pager
  // components hang /pg/<n> off the end of it
  const paginationBase = pathFilters
    ? joinBasePathWithFilters(basePath ?? "/", pathFilters, sortSlug)
    : (basePath ?? "/");
  const preservedParams: Record<string, string> = {};
  if (query && query.trim()) preservedParams.q = query.trim();
  if (!pathFilters) {
    for (const f of attributes) {
      preservedParams[f.slug] = [...f.values].sort().join(",");
    }
  }

  // Paging moved from ?page=N into the path (/pg/N) - anything still arriving with the old
  // query param is an external bookmark or a stale index entry, so send it to the canonical
  // path form BEFORE fetching, rather than silently serving page 1 under a duplicate URL
  const legacyPage = parsePage(searchParams);
  if (legacyPage > 1 && basePath) {
    // hrefForPage carries q and friends across, so nothing is dropped by the move
    const target = hrefForPage(paginationBase, preservedParams, legacyPage);
    permanentRedirect(await withLocalePrefix(target, await getLocale()));
  }

  // load_more/hybrid render ?page=N cumulatively (items 1..N, one slice query per page,
  // capped) so the Load more control can be a plain link - past the cap, and always in
  // pagination mode, ?page=N is the classic single window
  const style = await getPlpPaginationStyle();
  const plan =
    style === "pagination"
      ? { cumulative: false as const, offset: (currentPage - 1) * pageSize }
      : planSlices(currentPage, pageSize);
  const offset = plan.cumulative ? 0 : plan.offset;

  const varsFor = (sliceOffset: number) => ({
    query: query && query.trim() ? query : null,
    filter: {
      ...(filter ?? {}),
      attributes: attributes.length > 0 ? attributes : null,
    },
    sort: sortSlug || null,
    limit: pageSize,
    offset: sliceOffset,
  });

  // The offset-0 slice uses the full query: facets aggregate over the whole filtered set,
  // so they're offset-independent and only need fetching once - the slim slices skip them
  const [head, sortOptions, ...restSlices] = await Promise.all([
    browseProducts(varsFor(plan.cumulative ? 0 : plan.offset)),
    productSortOptions(),
    ...(plan.cumulative
      ? plan.offsets.slice(1).map((o) => browseProductsSlim(varsFor(o)))
      : []),
  ]);
  const data = plan.cumulative
    ? {
        ...head.data,
        items: mergeSlices([
          head.data.items,
          ...restSlices.map((s) => s.data.items as typeof head.data.items),
        ]),
      }
    : head.data;

  // Backend returns facets in ascending sortOrder, then name - render in receive order
  // rather than re-sorting locally
  // Identity of "which results are these" - a change here is what the crossfade animates between
  const gridKey = [
    query?.trim() ?? "",
    sortSlug,
    (attributes ?? []).map((a) => `${a.slug}:${a.values.join("+")}`).join(","),
    plan.cumulative ? "" : currentPage,
  ].join("|");

  const facetInputs: FacetGroupInput[] = data.facets.map((f) => ({
    slug: f.slug,
    label: f.name,
    description: f.description,
    defaultCollapsed: f.defaultCollapsed,
    buckets: f.buckets,
  }));

  const filterLabel = describeFilters(attributes, data.facets);
  // Stamp affinity signals (attr:<slug>) for any flagged attribute among the active filters - using value labels (not
  // URL slugs) so they aggregate with the PDP-view affinities on the Synapse profile
  const flagged = await synapseTrackedSlugs();
  const affinityProps: Record<string, string> = {};
  for (const a of attributes) {
    if (!flagged.has(a.slug)) continue;
    const facet = data.facets.find((f) => f.slug === a.slug);
    const labels = a.values.map((vs) => facet?.buckets.find((b) => b.slug === vs)?.value ?? vs);
    if (labels.length) affinityProps["attr:" + a.slug] = labels.join(", ");
  }

  // Every PLP (category, collection, search, /products) gets the ItemList through this one
  // shell - each entry pointing at the locale-canonical PDP URL, positions running across pages
  const [canonical, base] = await Promise.all([canonicalPathMapper(), getSiteUrl()]);

  return (
    <main className="mx-auto w-full max-w-8xl px-6 md:px-10 pt-6 pb-14">
      {data.items.length > 0 && (
        <JsonLd data={itemListJsonLd(data.items, (p) => canonical(productHref(p)), offset, base)} />
      )}
      {filterLabel && (
        <LensTrack
          key={filterLabel}
          event="product_filter"
          props={{ filters: filterLabel, ...affinityProps }}
        />
      )}
      {/* Lens search signal - /search only, keyed on the query so it fires once per term */}
      {query && query.trim() && (
        <LensTrack
          key={`search:${query.trim().toLowerCase()}`}
          event="$search"
          props={{ query: query.trim(), results: data.totalCount }}
        />
      )}
      {header}
      {prefix}

      {/* minmax(0, ...) stops intrinsic content pushing a column wider than its track
          The sidebar column only exists when there are facets to put in it: FacetSidebar renders
          NOTHING on a listing with none, and the products section then auto-placed into the 240px
          sidebar track and squashed the whole grid into it */}
      <div
        className={`mt-10 grid grid-cols-1 gap-10 md:items-start ${
          facetInputs.length > 0 ? "md:grid-cols-[minmax(0,240px)_minmax(0,1fr)]" : ""
        }`}
      >
        <FacetSidebar
          facets={facetInputs}
          basePath={basePath}
          pathFilters={pathFilters}
          pathSort={sortSlug}
          facetRules={await (async () => {
            const seo = await getSeoConfig();
            return { combinations: seo.facetCombinations, minResults: seo.minFacetResults };
          })()}
        />

        <section className="min-w-0">
          <div className="flex flex-wrap items-baseline justify-between gap-4">
            <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              Products
            </h2>
            <div className="flex items-center gap-4">
              <SortSelect
                options={sortOptions}
                currentSlug={sortSlug}
                basePath={basePath}
                pathFilters={pathFilters}
              />
              <p className="text-xs text-muted-foreground">
                {data.totalCount} match{data.totalCount === 1 ? "" : "es"}
              </p>
            </div>
          </div>
          {data.items.length === 0 ? (
            <p className="mt-3 text-sm text-muted-foreground">{emptyText}</p>
          ) : (
            <>
              {/* Re-filtering, re-sorting or paging swaps the grid's contents while everything around
                  it stays put, so it crossfades rather than hard-cutting - "same place, different
                  results". The key is what makes React treat the two grids as an exit/enter pair
                  instead of an in-place update
                  Cumulative load-more is excluded from the key on purpose: it APPENDS to the list, so
                  keying on the page would crossfade rows the shopper is already reading */}
              <ViewTransition
                key={gridKey}
                name="product-grid"
                share="auto"
                enter="auto"
                // Leaving for a PDP must stay the image's moment - without this the grid would
                // animate on that navigation too and compete with the morph
                default="none"
              >
                <ul className="mt-4 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
                  {data.items.map((p: BrowseProductsItem) => (
                    <li key={p.id}>
                      {/* BrowseProductsItem ⊇ ProductSummary at runtime; the
                          generated/hand-written types drift on nested shapes. */}
                      <ProductCard
                        product={p as unknown as ProductSummary}
                        morphToDetail
                      />
                    </li>
                  ))}
                </ul>
              </ViewTransition>
              {(style !== "pagination" && plan.cumulative) && (
                <LoadMore
                  basePath={paginationBase}
                  preservedParams={preservedParams}
                  currentPage={currentPage}
                  pageSize={pageSize}
                  shownCount={data.items.length}
                  totalCount={data.totalCount}
                />
              )}
              {/* hybrid keeps the numbered pager under the button so crawlers and deep
                  links lose nothing; past the cumulative cap load_more degrades to the
                  classic pager too, since a lone Load more would imply state that isn't there */}
              {(style === "pagination" || style === "hybrid" || !plan.cumulative) && (
                <Pagination
                  totalCount={data.totalCount}
                  pageSize={pageSize}
                  currentPage={currentPage}
                  basePath={paginationBase}
                  preservedParams={preservedParams}
                />
              )}
            </>
          )}
        </section>
      </div>

      {suffix}
    </main>
  );
}
