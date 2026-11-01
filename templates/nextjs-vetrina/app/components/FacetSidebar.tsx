"use client";

import { Link, useRouter, usePathname } from "@/app/components/LocaleLink";
import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";

import {
  filterChainAllowed,
  joinBasePathWithFilters,
  toggleFilterValue,
  type FacetIndexRules,
} from "@skelet/storefront-core/filter-path";

export type FacetGroupInput = {
  slug: string;
  // Human-readable label rendered above the bucket list - sourced from the catalog
  // attribute's name on the backend
  label: string;
  description: string;
  // Backend hint: when true, the panel renders collapsed on first paint
  defaultCollapsed: boolean;
  buckets: {
    value: string;
    slug: string;
    isIndexable: boolean;
    count: number;
  }[];
};

const RESERVED_QUERY_KEYS = new Set(["q", "page"]);

function withoutPagination(qs: URLSearchParams): URLSearchParams {
  const next = new URLSearchParams(qs.toString());
  next.delete("page");
  return next;
}

// Query-string href builders - the fallback for /products and any catch-all entity page
// that didn't pre-parse a filter chain
function hrefForToggleQuery(
  pathname: string,
  current: URLSearchParams,
  attrSlug: string,
  bucketSlug: string,
): string {
  const next = withoutPagination(current);
  const raw = next.get(attrSlug);
  const values = raw ? raw.split(",").filter(Boolean) : [];
  const has = values.includes(bucketSlug);
  const nextValues = has
    ? values.filter((v) => v !== bucketSlug)
    : [...values, bucketSlug];
  if (nextValues.length === 0) next.delete(attrSlug);
  else next.set(attrSlug, nextValues.sort().join(","));
  const qs = next.toString();
  return qs ? `${pathname}?${qs}` : pathname;
}

function hrefForClearQuery(pathname: string, current: URLSearchParams): string {
  const next = new URLSearchParams(current.toString());
  for (const k of Array.from(next.keys())) {
    if (!RESERVED_QUERY_KEYS.has(k)) next.delete(k);
  }
  const qs = next.toString();
  return qs ? `${pathname}?${qs}` : pathname;
}

export function FacetSidebar({
  facets,
  basePath,
  pathFilters,
  pathSort,
  facetRules = { combinations: [], minResults: 0 },
  currentQuery = "",
}: {
  facets: FacetGroupInput[];
  basePath?: string;
  pathFilters?: { slug: string; values: string[] }[];
  // Active sort slug to thread through the rebuilt URLs so toggling a filter or
  // clearing all filters doesn't drop the user's chosen sort
  pathSort?: string;
  // seo/facets settings - a facet link renders as a crawlable <a> only when its TARGET
  // chain is an allowed combination of indexable values with enough results; everything
  // else renders as a button so crawlers never enter the combination loop
  facetRules?: FacetIndexRules & { minResults: number };
  // The page's query string, handed down by the server - useSearchParams here would bail the
  // force-static catch-all out to client rendering, leaving no-JS visitors a blank listing
  currentQuery?: string;
}) {
  const pathname = usePathname();
  const t = useTranslations("listing.facets");
  const current = useMemo(() => new URLSearchParams(currentQuery), [currentQuery]);

  const usePath = basePath != null && pathFilters != null;

  const selected = useMemo(() => {
    const out = new Map<string, Set<string>>();
    if (usePath) {
      for (const f of pathFilters!) {
        out.set(f.slug, new Set(f.values));
      }
      return out;
    }
    for (const [k, v] of current.entries()) {
      if (RESERVED_QUERY_KEYS.has(k)) continue;
      out.set(k, new Set(v.split(",").filter(Boolean)));
    }
    return out;
  }, [usePath, pathFilters, current]);

  const hasSelection = selected.size > 0;
  const sortForHrefs = pathSort ?? "";

  const carriedQs = withoutPagination(current).toString();

  const clearHref = usePath
    ? joinBasePathWithFilters(basePath!, [], sortForHrefs, carriedQs)
    : hrefForClearQuery(pathname, current);

  function toggleHref(attrSlug: string, bucketSlug: string): string {
    if (usePath) {
      const next = toggleFilterValue(pathFilters!, attrSlug, bucketSlug);
      return joinBasePathWithFilters(basePath!, next, sortForHrefs, carriedQs);
    }
    return hrefForToggleQuery(pathname, current, attrSlug, bucketSlug);
  }

  // Would the page a toggle lands on still be indexable? Mirrors
  // evaluateFilterIndexability: every value in the RESULTING chain must be an
  // indexable bucket and the total value count must stay under the cap
  const indexableValueSlugs = useMemo(() => {
    const out = new Map<string, Set<string>>();
    for (const g of facets) {
      out.set(g.slug, new Set(g.buckets.filter((b) => b.isIndexable).map((b) => b.slug)));
    }
    return out;
  }, [facets]);

  function targetIndexable(attrSlug: string, bucketSlug: string): boolean {
    const chain = [...selected.entries()].map(([slug, values]) => ({
      slug,
      values: [...values],
    }));
    const next = toggleFilterValue(chain, attrSlug, bucketSlug);
    if (!filterChainAllowed(next, facetRules)) return false;
    return next.every((f) => f.values.every((v) => indexableValueSlugs.get(f.slug)?.has(v)));
  }

  const activeChips: ChipDescriptor[] = useMemo(() => {
    if (!hasSelection) return [];
    const out: ChipDescriptor[] = [];
    const groupBySlug = new Map(facets.map((g) => [g.slug, g]));
    // Chips come from the SELECTION, not from the returned buckets. A terms agg
    // omits zero-count values, so combining two filters that share no products
    // dropped the chip for the filter that emptied the page - the shopper saw no
    // results, no chip, and no way back except "Clear all"
    for (const [groupSlug, values] of selected) {
      const group = groupBySlug.get(groupSlug);
      for (const valueSlug of values) {
        const bucket = group?.buckets.find((b) => b.slug === valueSlug);
        out.push({
          groupLabel: group?.label ?? groupSlug,
          // No bucket means the value returned nothing - show the slug so it stays removable
          value: bucket?.value ?? valueSlug,
          href: toggleHref(groupSlug, valueSlug),
          // Removal target, not this value: a chip's link points at the chain minus itself
          isIndexable: targetIndexable(groupSlug, valueSlug),
        });
      }
    }
    return out;
    // toggleHref closes over carriedQs / pathFilters / pathname etc; they're all
    // derived from the same hook inputs so a single deps array entry on `selected` +
    // `facets` is enough to stay coherent
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [facets, selected, hasSelection]);

  if (facets.length === 0) {
    return null;
  }

  return (
    <>
      <input
        id="facet-drawer-toggle"
        type="checkbox"
        className="peer/drawer sr-only"
        aria-label={t("toggle")}
      />

      {/* Mobile only - bottom-right keeps it in thumb reach and clear of the breadcrumb row */}
      <label
        htmlFor="facet-drawer-toggle"
        className="fixed bottom-5 right-5 z-40 inline-flex cursor-pointer items-center gap-2 rounded-md bg-primary px-5 py-3 text-sm font-semibold text-primary-foreground shadow-lg shadow-black/20 md:hidden"
      >
        <FilterIcon />
        {t("filters")}
        {hasSelection ? (
          <span className="rounded-sm bg-white/25 px-1.5 py-0.5 text-[10px] tabular-nums text-white">
            {activeChips.length}
          </span>
        ) : null}
      </label>

      {/* Below the aside in the z-stack so the aside still catches its own taps */}
      <label
        htmlFor="facet-drawer-toggle"
        aria-label={t("close")}
        // Same scrim as dialog.drawer::backdrop and the search modal - this one is a peer-checked
        // label rather than a <dialog>, so it can't inherit it
        className="pointer-events-none fixed inset-0 z-40 bg-black/10 opacity-0 backdrop-blur-[2px] transition-opacity peer-checked/drawer:pointer-events-auto peer-checked/drawer:opacity-100 md:hidden"
      />

      {/* Bottom sheet on mobile, sticky column on md+ - the peer-checked/drawer utilities
          only fire from the hidden checkbox, so no JS drives the open state */}
      <aside
        className="
          scrollbar-on-hover
          fixed inset-x-0 bottom-0 z-50 flex h-[82dvh] min-w-0 translate-y-full flex-col gap-5 overflow-y-auto rounded-t-2xl bg-card p-5 shadow-2xl shadow-black/30 transition-transform duration-300 ease-out
          peer-checked/drawer:translate-y-0
          md:sticky md:inset-auto md:top-24 md:z-auto md:h-auto md:max-h-[calc(100vh-7rem)] md:translate-y-0 md:self-start md:rounded-none md:bg-transparent md:p-0 md:pr-2 md:shadow-none md:transition-none
        "
      >
        {/* Visual only - this panel's open state is a CSS checkbox, so nothing to drag */}
        <div aria-hidden="true" className="-mt-2 flex shrink-0 justify-center pb-1 md:hidden">
          <span className="h-1 w-9 rounded-full bg-foreground/25" />
        </div>
        <div className="flex items-center justify-between border-b border-border pb-3">
          <h2 className="flex items-center gap-2 text-sm font-medium">
            <FilterIcon />
            {t("filters")}
            {hasSelection ? (
              <span className="rounded-sm bg-foreground px-2 py-0.5 text-[10px] font-semibold text-background">
                {activeChips.length}
              </span>
            ) : null}
          </h2>
          <div className="flex items-center gap-2">
            {hasSelection ? (
              <Link
                href={clearHref}
                scroll={false}
                prefetch={false}
                className="text-xs text-muted-foreground underline-offset-2 hover:underline"
              >
                {t("clearAll")}
              </Link>
            ) : null}
            {/* Drawer close - mobile only. The label flips the
                checkbox back off so the drawer slides away. */}
            <label
              htmlFor="facet-drawer-toggle"
              className="grid size-7 cursor-pointer place-items-center rounded-md text-muted-foreground hover:bg-muted md:hidden"
              aria-label={t("close")}
            >
              <CloseIcon />
            </label>
          </div>
        </div>

        {activeChips.length > 0 ? (
          <ActiveChipsRow chips={activeChips} clearHref={clearHref} />
        ) : null}

        <div className="grid divide-y divide-border">
          {facets.map((group) => {
              const groupSelected =
                selected.get(group.slug) ?? new Set<string>();
              const open = groupSelected.size > 0 || !group.defaultCollapsed;
              return (
                // <details> drives the disclosure: zero-JS, SSR-friendly, and resets correctly on every navigation
                // (each render is a fresh tree, so the `open` attribute is the source of truth)
                <details
                  key={group.slug}
                  open={open}
                  className="group min-w-0 [&_summary::-webkit-details-marker]:hidden"
                >
                  <summary className="flex cursor-pointer list-none items-center justify-between gap-2 py-3 text-sm font-medium text-foreground">
                    <span className="flex min-w-0 items-baseline gap-2">
                      <span className="min-w-0 truncate">{group.label}</span>
                      {groupSelected.size > 0 ? (
                        <span className="shrink-0 rounded-sm bg-foreground px-1.5 py-0.5 text-[10px] font-semibold text-background">
                          {groupSelected.size}
                        </span>
                      ) : null}
                    </span>
                    {/* Chevron rotates via the parent <details>'s [open] state. */}
                    <svg
                      aria-hidden="true"
                      viewBox="0 0 12 12"
                      className="size-3 shrink-0 text-muted-foreground transition-transform group-open:rotate-180"
                    >
                      <path
                        d="M3 4.5l3 3 3-3"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="1.5"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      />
                    </svg>
                  </summary>
                  {group.description && (
                    <p className="mb-2 text-xs leading-snug text-muted-foreground">
                      {group.description}
                    </p>
                  )}
                  <FacetValueList
                    group={group}
                    groupSelected={groupSelected}
                    hrefFor={(bucketSlug) => toggleHref(group.slug, bucketSlug)}
                    crawlableFor={(bucketSlug) => targetIndexable(group.slug, bucketSlug)}
                    minResults={facetRules.minResults}
                  />
                </details>
              );
            })}
        </div>
      </aside>
    </>
  );
}
// Long value lists fold behind "Show all" and gain a search box. Beyond-limit rows are
// HIDDEN, not unmounted - indexable facet links must stay in the HTML for crawlers (the
// SEO facet pages depend on them), the fold is purely visual
const VISIBLE_LIMIT = 8;
const SEARCH_MIN = 13;

function FacetValueList({
  group,
  groupSelected,
  hrefFor,
  crawlableFor,
  minResults,
}: {
  group: FacetGroupInput;
  groupSelected: Set<string>;
  hrefFor: (bucketSlug: string) => string;
  // Anchor vs button per bucket - decided on the toggle TARGET's indexability, not the
  // bucket's own flag, so unlisted combination pages never get crawlable links
  crawlableFor: (bucketSlug: string) => boolean;
  // Thin targets (fewer results than seo/facets/min_results) also stay buttons
  minResults: number;
}) {
  const router = useRouter();
  const t = useTranslations("listing.facets");
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState(false);

  const q = query.trim().toLowerCase();
  const searchable = group.buckets.length >= SEARCH_MIN;
  const foldable = group.buckets.length > VISIBLE_LIMIT;

  // A row survives the fold when it's within the limit, selected, or matches the search;
  // while a search is typed the fold is suspended entirely
  const matchCount = q
    ? group.buckets.filter((b) => b.value.toLowerCase().includes(q)).length
    : group.buckets.length;

  return (
    <div className="grid gap-1 pb-3">
      {searchable && (
        <div className="relative mb-1">
          <svg
            aria-hidden
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
          >
            <circle cx="11" cy="11" r="7" />
            <path d="M21 21l-4.3-4.3" />
          </svg>
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("searchPlaceholder", { group: group.label.toLowerCase() })}
            aria-label={t("searchLabel", { group: group.label })}
            className="h-8 w-full rounded-md border border-border bg-background pl-8 pr-2.5 text-sm outline-none transition placeholder:text-muted-foreground focus:border-foreground/50"
          />
        </div>
      )}

      <ul className="grid gap-0.5">
        {group.buckets.map((b, i) => {
          // URL state - and therefore `groupSelected` - keys off the bucket
          // slug, not the display value
          const checked = groupSelected.has(b.slug);
          const hiddenByFold =
            !q && foldable && !expanded && i >= VISIBLE_LIMIT && !checked;
          const hiddenBySearch = !!q && !b.value.toLowerCase().includes(q);
          const href = hrefFor(b.slug);
          const sharedClass = `flex w-full min-w-0 cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-left text-sm transition-colors ${
            checked
              ? "bg-foreground/5 font-medium text-foreground"
              : "text-foreground hover:bg-muted"
          }`;
          const inner = (
            <>
              {/* Native input stays rendered for screen readers, this is just the visible control */}
              <span
                aria-hidden
                className={`grid size-4 shrink-0 place-items-center rounded border transition-colors ${
                  checked
                    ? "border-foreground bg-foreground text-background"
                    : "border-border bg-transparent"
                }`}
              >
                {checked ? (
                  <svg
                    viewBox="0 0 12 12"
                    className="size-2.5"
                    aria-hidden
                  >
                    <path
                      d="M2.5 6.5l2.5 2.5 4.5-5"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    />
                  </svg>
                ) : null}
              </span>
              <input
                type="checkbox"
                checked={checked}
                readOnly
                tabIndex={-1}
                className="sr-only"
                aria-label={b.value}
              />
              {/* min-w-0 on the wrapping link is what makes the child's truncate legal */}
              <span className="min-w-0 flex-1 truncate">
                {b.value}
              </span>
              <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                {b.count}
              </span>
            </>
          );
          return (
            <li
              key={b.slug}
              className="min-w-0"
              hidden={hiddenByFold || hiddenBySearch}
            >
              {crawlableFor(b.slug) && (minResults === 0 || b.count >= minResults) ? (
                <Link
                  href={href}
                  scroll={false}
                  prefetch={false}
                  title={b.value}
                  className={`min-w-0 ${sharedClass}`}
                >
                  {inner}
                </Link>
              ) : (
                <button
                  type="button"
                  onClick={() =>
                    router.push(href, { scroll: false })
                  }
                  title={b.value}
                  className={`min-w-0 ${sharedClass}`}
                >
                  {inner}
                </button>
              )}
            </li>
          );
        })}
      </ul>

      {q && matchCount === 0 && (
        <p className="px-2 py-1.5 text-xs text-muted-foreground">
          {t("noMatch", { query: query.trim() })}
        </p>
      )}

      {!q && foldable && (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
          className="flex w-fit items-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <span
            aria-hidden
            className="grid size-3.5 place-items-center rounded-sm border border-current"
          >
            <svg
              viewBox="0 0 10 10"
              className="size-2.5"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
            >
              {expanded ? (
                <path d="M2.5 5h5" />
              ) : (
                <path d="M5 2.5v5M2.5 5h5" />
              )}
            </svg>
          </span>
          {expanded ? t("showFewer") : t("showAll", { count: group.buckets.length })}
        </button>
      )}
    </div>
  );
}

type ChipDescriptor = {
  groupLabel: string;
  value: string;
  href: string;
  isIndexable: boolean;
};
function ActiveChipsRow({
  chips,
  clearHref,
}: {
  chips: ChipDescriptor[];
  clearHref: string;
}) {
  const t = useTranslations("listing.facets");
  return (
    <div className="grid gap-2">
      <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        {t("active")}
      </p>
      <ul className="flex flex-wrap gap-1.5">
        {chips.map((c, i) => (
          <li key={`${c.groupLabel}-${c.value}-${i}`}>
            {c.isIndexable ? (
              <Link
                href={c.href}
                scroll={false}
                prefetch={false}
                title={t("removeChip", { group: c.groupLabel, value: c.value })}
                className="inline-flex items-center gap-1 rounded-md bg-foreground px-2.5 py-1 text-xs text-background transition-colors hover:bg-foreground/90"
              >
                <span className="text-muted-foreground">
                  {c.groupLabel}
                </span>
                <span>{c.value}</span>
                <CloseIcon />
              </Link>
            ) : (
              <ChipButton chip={c} />
            )}
          </li>
        ))}
        <li>
          <Link
            href={clearHref}
            scroll={false}
            prefetch={false}
            className="inline-flex items-center gap-1 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent"
          >
            {t("clearAll")}
          </Link>
        </li>
      </ul>
    </div>
  );
}

function ChipButton({ chip }: { chip: ChipDescriptor }) {
  const router = useRouter();
  const t = useTranslations("listing.facets");
  return (
    <button
      type="button"
      onClick={() => router.push(chip.href, { scroll: false })}
      title={t("removeChip", { group: chip.groupLabel, value: chip.value })}
      className="inline-flex items-center gap-1 rounded-md bg-foreground px-2.5 py-1 text-xs text-background transition-colors hover:bg-foreground/90"
    >
      <span className="text-muted-foreground">
        {chip.groupLabel}
      </span>
      <span>{chip.value}</span>
      <CloseIcon />
    </button>
  );
}

function FilterIcon() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M3 6h18" />
      <path d="M7 12h10" />
      <path d="M10 18h4" />
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg
      width="10"
      height="10"
      viewBox="0 0 12 12"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M3 3l6 6M9 3l-6 6" />
    </svg>
  );
}
