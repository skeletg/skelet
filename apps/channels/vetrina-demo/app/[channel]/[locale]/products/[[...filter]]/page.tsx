import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { notFound } from "next/navigation";

import { ProductListShell } from "@/app/components/ProductListShell";
import { localizedAlternates } from "@/app/lib/locale/alternates";

import { browseProducts, productSortOptions } from "@/app/lib/catalog/read";
import { getBrowsePageSize, getSeoConfig } from "@/app/lib/config/read";
import {
  isPagedUrlIndexable,
  parsePageFromSegments,
  withPageSegment,
  joinBasePathWithFilters,
  assembleH1,
  evaluateFilterIndexability,
  isChainStart,
  isSortUrlIndexable,
  parseFilterSegments,
  parseSortFromSegments,
} from "@/app/lib/filter-path";

type RawSearchParams = Record<string, string | string[] | undefined>;

async function indexabilityFor(
  pathFilters: { slug: string; values: string[] }[],
) {
  const { data } = await browseProducts({
    filter: {
      attributes: pathFilters.length > 0 ? pathFilters : null,
    },
    limit: await getBrowsePageSize(),
    offset: 0,
  });
  const seo = await getSeoConfig();
  const idx = evaluateFilterIndexability(pathFilters, data.facets, {
    combinations: seo.facetCombinations,
  });
  // Thin-page guard - only for FILTERED pages, a sparse base listing stays indexable
  if (
    pathFilters.length > 0 &&
    seo.minFacetResults > 0 &&
    data.totalCount < seo.minFacetResults
  ) {
    idx.indexable = false;
  }
  return idx;
}

export async function generateMetadata(
  props: PageProps<"/[channel]/[locale]/products/[[...filter]]">,
): Promise<Metadata> {
  const { filter, locale } = await props.params;
  const t = await getTranslations({ locale, namespace: "listing" });
  const pathFilters = filter ? parseFilterSegments(filter) : [];
  const sortSlug = filter ? parseSortFromSegments(filter) : "";
  const [{ indexable: filtersIndexable }, sortOpts] = await Promise.all([
    indexabilityFor(pathFilters),
    productSortOptions(),
  ]);
  const sortIndexable = isSortUrlIndexable(sortSlug, sortOpts);
  const page = parsePageFromSegments(filter ?? []);
  // See CategoryView: the filtered axis is capped at page 1 by default
  const { pageIndexCaps, robotsNoindex, robotsIndexable } = await getSeoConfig();
  const indexable =
    filtersIndexable &&
    sortIndexable &&
    isPagedUrlIndexable(page, pathFilters.length > 0, pageIndexCaps);
  const href = withPageSegment(joinBasePathWithFilters("/products", pathFilters, sortSlug), page);
  return {
    title: page > 1 ? t("paged", { name: t("productsTitle"), page }) : t("productsTitle"),
    robots: indexable ? robotsIndexable : robotsNoindex,
    alternates: await localizedAlternates(href),
  };
}

export default async function ProductsPage(
  props: PageProps<"/[channel]/[locale]/products/[[...filter]]">,
) {
  const { filter, locale } = await props.params;
  const t = await getTranslations({ locale, namespace: "listing" });
  const searchParams = (await props.searchParams) as RawSearchParams;

// Owns all of /products/* - valid paths are empty or an /f/../v/../s/ chain A bare segment like /products/<UUID> is a
// stale id URL, so 404 rather than render the unfiltered listing under a misleading URL
  if (filter && filter.length > 0 && !isChainStart(filter[0])) {
    notFound();
  }
  const pathFilters = filter ? parseFilterSegments(filter) : [];
  const sortSlug = filter ? parseSortFromSegments(filter) : "";
  const [idx, sortOpts] = await Promise.all([
    indexabilityFor(pathFilters),
    productSortOptions(),
  ]);
  const sortIndexable = isSortUrlIndexable(sortSlug, sortOpts);
  const effective = sortIndexable ? idx : { ...idx, indexable: false };
  const displayName = assembleH1(t("productsTitle"), effective);
  const valueDescriptions = effective.indexable
    ? effective.values.map((v) => v.description).filter(Boolean)
    : [];

  return (
    <ProductListShell
      searchParams={searchParams}
      pathFilters={pathFilters}
      pathSort={sortSlug}
      pathPage={parsePageFromSegments(filter ?? [])}
      basePath="/products"
      emptyText={t("noProductsYet")}
      header={
        <>
          <h1 className="text-3xl font-medium tracking-tight">{displayName}</h1>
          {valueDescriptions.length > 0 && (
            <div className="mt-2 max-w-2xl space-y-2 text-sm text-muted-foreground">
              {valueDescriptions.map((d, i) => (
                <p key={i}>{d}</p>
              ))}
            </div>
          )}
        </>
      }
    />
  );
}
