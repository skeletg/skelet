import type { Metadata } from "next";
import { canonicalPathMapper, localizedAlternates } from "@/app/lib/locale/alternates";
import { notFound } from "next/navigation";

import { Breadcrumbs } from "@/app/components/Breadcrumbs";
import { JsonLd } from "@/app/components/JsonLd";
import { ProductListShell } from "@/app/components/ProductListShell";
import { browseProducts, getCollection, productSortOptions } from "@/app/lib/catalog/read";
import { getSeoConfig } from "@/app/lib/config/read";
import { displayName } from "@/app/lib/display";
import { collectionHref } from "@/app/lib/entity-href";
import {
  isPagedUrlIndexable,
  parsePageFromSegments,
  withPageSegment,
  assembleH1,
  evaluateFilterIndexability,
  isSortUrlIndexable,
  parseFilterSegments,
  parseSortFromSegments,
} from "@/app/lib/filter-path";
import { breadcrumbJsonLd } from "@/app/lib/json-ld";

import { LexicalRichText, lexicalHasContent } from "@/app/lib/lexical-render";

async function indexabilityFor(
  collectionId: string,
  pathFilters: { slug: string; values: string[] }[],
) {
  const { data } = await browseProducts({
    filter: {
      collectionId,
      attributes: pathFilters.length > 0 ? pathFilters : null,
    },
    limit: 24,
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
  props: {
    params: Promise<{ id: string; filter?: string[]; basePath?: string }>;
    searchParams: Promise<Record<string, string | string[] | undefined>>;
  },
): Promise<Metadata> {
  const rawParams = (await props.params) as {
    id: string;
    filter?: string[];
  };
  const { id, filter } = rawParams;
  const { collection } = await getCollection(id);
  if (!collection) return { title: "Collection not found" };
  const title = displayName(collection.metaTitle ?? collection.name, collection.slug);
  const pathFilters = filter ? parseFilterSegments(filter) : [];
  const sortSlug = filter ? parseSortFromSegments(filter) : "";
  const [{ indexable: filtersIndexable }, sortOpts] = await Promise.all([
    indexabilityFor(id, pathFilters),
    productSortOptions(),
  ]);
  const page = parsePageFromSegments(filter ?? []);
  // See CategoryView: the filtered axis is capped at page 1 by default
  const { pageIndexCaps, robotsNoindex, robotsIndexable } = await getSeoConfig();
  const indexable =
    filtersIndexable &&
    isSortUrlIndexable(sortSlug, sortOpts) &&
    isPagedUrlIndexable(page, pathFilters.length > 0, pageIndexCaps);
  const href = withPageSegment(collectionHref(collection, pathFilters), page);
  return {
    title: page > 1 ? `${title} - page ${page}` : title,
    description: collection.metaDescription ?? undefined,
    robots: indexable ? robotsIndexable : robotsNoindex,
    alternates: await localizedAlternates(href),
  };
}

export default async function CollectionPage(
  props: {
    params: Promise<{ id: string; filter?: string[]; basePath?: string }>;
    searchParams: Promise<Record<string, string | string[] | undefined>>;
  },
) {
  const rawParams = (await props.params) as {
    id: string;
    filter?: string[];
    basePath?: string;
  };
  const { id, filter, basePath: prettyBasePath } = rawParams;
  const searchParams = (await props.searchParams) as Record<
    string,
    string | string[] | undefined
  >;
  const { collection } = await getCollection(id);
  if (!collection) notFound();

  const baseName = displayName(collection.name, collection.slug);
  const pathFilters = filter ? parseFilterSegments(filter) : [];
  const sortSlug = filter ? parseSortFromSegments(filter) : "";
  const basePath = prettyBasePath ?? `/collections/${id}`;
  const [idx, sortOpts] = await Promise.all([
    indexabilityFor(id, pathFilters),
    productSortOptions(),
  ]);
  const sortIndexable = isSortUrlIndexable(sortSlug, sortOpts);
  const effective = sortIndexable ? idx : { ...idx, indexable: false };
  const heading = assembleH1(baseName, effective);
  const valueDescriptions = effective.indexable
    ? effective.values.map((v) => v.description).filter(Boolean)
    : [];

  // Drop the synthetic "Collections" root so the trail matches the
  // product/category breadcrumbs: Home / <this collection>.
  const breadcrumbItems = [
    { name: "Home", href: "/" },
    { name: baseName, href: collectionHref(collection) },
  ];
  const canonical = await canonicalPathMapper();

  return (
    <ProductListShell
      filter={{ collectionId: id }}
      searchParams={searchParams}
      pathFilters={pathFilters}
      pathSort={sortSlug}
      pathPage={parsePageFromSegments(filter ?? [])}
      basePath={basePath}
      emptyText="No products match the current filters in this collection."
      header={
        <>
          <JsonLd
            data={breadcrumbJsonLd(
              breadcrumbItems.map((it) => ({ ...it, href: canonical(it.href) })),
            )}
          />
          <Breadcrumbs items={breadcrumbItems} />
          <header className="mt-6 flex items-center gap-4">
            {collection.coverImage && (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={collection.coverImage.url}
                alt={collection.coverImage.altText || heading}
                className="h-16 w-16 rounded-2xl object-cover"
              />
            )}
            <div>
              <p className="text-xs uppercase tracking-wider text-muted-foreground">
                Collection
              </p>
              <h1 className="mt-1 text-3xl font-medium tracking-tight">
                {heading}
              </h1>
            </div>
          </header>
          {valueDescriptions.length > 0 && (
            <div className="mt-4 max-w-2xl space-y-2 text-sm text-muted-foreground">
              {valueDescriptions.map((d, i) => (
                <p key={i}>{d}</p>
              ))}
            </div>
          )}
        </>
      }
      suffix={
        lexicalHasContent(collection.description) ? (
          <section className="mt-12 max-w-3xl border-t border-border pt-8">
            <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              About {heading}
            </h2>
            <div className="mt-3 space-y-4 text-sm leading-relaxed text-foreground">
              <LexicalRichText value={collection.description} />
            </div>
          </section>
        ) : null
      }
    />
  );
}
