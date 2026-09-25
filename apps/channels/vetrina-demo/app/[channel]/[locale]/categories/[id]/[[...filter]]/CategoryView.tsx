import type { Metadata } from "next";
import { getSiteUrl } from "@/app/lib/channel/site";
import { canonicalPathMapper, localizedAlternates } from "@/app/lib/locale/alternates";
import { Link } from "@/app/components/LocaleLink";
import { notFound } from "next/navigation";

import { Breadcrumbs } from "@/app/components/Breadcrumbs";
import { JsonLd } from "@/app/components/JsonLd";
import { ProductListShell } from "@/app/components/ProductListShell";
import { browseProducts, getCategory, productSortOptions } from "@/app/lib/catalog/read";
import { getSeoConfig } from "@/app/lib/config/read";
import { displayName } from "@/app/lib/display";
import { categoryHref } from "@/app/lib/entity-href";
import { breadcrumbJsonLd } from "@/app/lib/json-ld";
import {
  isPagedUrlIndexable, parsePageFromSegments, withPageSegment } from "@/app/lib/filter-path";
import { absUrl } from "@/app/lib/sitemap/read";
import {
  assembleH1,
  evaluateFilterIndexability,
  isSortUrlIndexable,
  parseFilterSegments,
  parseSortFromSegments,
} from "@/app/lib/filter-path";
import { Description } from "@/app/[channel]/[locale]/products/[id]/parts";
import { LexicalRichText, lexicalHasContent } from "@/app/lib/lexical-render";

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
  const { category } = await getCategory(id);
  if (!category) return { title: "Category not found" };
  const pathFilters = filter ? parseFilterSegments(filter) : [];
  const sortSlug = filter ? parseSortFromSegments(filter) : "";
  const [{ indexable: filtersIndexable }, sortOpts] = await Promise.all([
    evaluateIndexability({ categoryId: id }, pathFilters),
    productSortOptions(),
  ]);
  const page = parsePageFromSegments(filter ?? []);
  // Filters and pages multiply, so a filtered listing is capped at page 1 unless the operator
  // raises seo/facets/max_indexable_page - pages past the cap stay noindex,FOLLOW
  const { pageIndexCaps, robotsNoindex, robotsIndexable } = await getSeoConfig();
  const indexable =
    filtersIndexable &&
    isSortUrlIndexable(sortSlug, sortOpts) &&
    isPagedUrlIndexable(page, pathFilters.length > 0, pageIndexCaps);
  const baseTitle = displayName(category.name, category.slug);
  const title = page > 1 ? `${baseTitle} - page ${page}` : baseTitle;
  // Page N self-canonicalizes WITH ?page=N (the blog pattern) - pointing it at page 1 would
  // hide the deeper products, and in load-more modes page 1 is a strict subset anyway
  const pagedHref = withPageSegment(categoryHref(category, pathFilters), page);
  return {
    title,
    // The verdict comes from the facet rules; the tag it turns into is configured under
    // Settings -> SEO -> Facet indexing (seo/facets/robots_*)
    robots: indexable ? robotsIndexable : robotsNoindex,
    alternates: await localizedAlternates(pagedHref),
    openGraph: { type: "website", title, url: absUrl(pagedHref, await getSiteUrl()) },
    twitter: { card: "summary", title },
  };
}

async function evaluateIndexability(
  baseFilter: { categoryId?: string; collectionId?: string },
  pathFilters: { slug: string; values: string[] }[],
) {
  const { data } = await browseProducts({
    filter: {
      ...baseFilter,
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

export default async function CategoryPage(
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
  const { category } = await getCategory(id);
  if (!category) notFound();

  const baseName = displayName(category.name, category.slug);
  const pathFilters = filter ? parseFilterSegments(filter) : [];
  const sortSlug = filter ? parseSortFromSegments(filter) : "";
  const basePath = prettyBasePath ?? `/categories/${id}`;
  // Reuse the cached browseProducts call from generateMetadata
  const [idx, sortOpts] = await Promise.all([
    evaluateIndexability({ categoryId: id }, pathFilters),
    productSortOptions(),
  ]);
  // Effective indexability folds in the sort URL - a non-indexable sort drops the H1
  // augmentation + SEO copy alongside the noindex robots meta from generateMetadata
  const sortIndexable = isSortUrlIndexable(sortSlug, sortOpts);
  const effective = sortIndexable ? idx : { ...idx, indexable: false };
  const heading = assembleH1(baseName, effective);
  const valueDescriptions = effective.indexable
    ? effective.values.map((v) => v.description).filter(Boolean)
    : [];

  // Drop the synthetic "Products" root and the top-level root category (ancestors[0], e.g. "Shop") so the trail matches
  // the PDP: Home / <ancestors below root> / <this category>
  const breadcrumbItems = [
    { name: "Home", href: "/" },
    ...category.ancestors.slice(1).map((a) => ({
      name: displayName(a.name, a.slug),
      href: categoryHref(a),
    })),
    { name: baseName, href: categoryHref(category) },
  ];
  const [canonical, base] = await Promise.all([canonicalPathMapper(), getSiteUrl()]);

  return (
    <ProductListShell
      filter={{ categoryId: id }}
      searchParams={searchParams}
      pathFilters={pathFilters}
      pathSort={sortSlug}
      pathPage={parsePageFromSegments(filter ?? [])}
      basePath={basePath}
      emptyText="No products match the current filters in this category."
      header={
        <>
          <JsonLd
            data={breadcrumbJsonLd(
              breadcrumbItems.map((it) => ({ ...it, href: canonical(it.href) })),
              base,
            )}
          />
          <Breadcrumbs items={breadcrumbItems} className="text-sm mb-3" />
          <p className="text-xs uppercase tracking-wider text-muted-foreground">
            Category
          </p>
          {/* heading already carries the filter segments when the values are indexable */}
          <h1 className="mt-1 text-3xl font-medium tracking-tight">
            {heading}
          </h1>
          <p className="mt-1 text-xs text-muted-foreground">/{category.urlPath}</p>
          {valueDescriptions.length > 0 && (
            <div className="mt-4 max-w-2xl space-y-2 text-sm text-muted-foreground">
              {valueDescriptions.map((d, i) => (
                <p key={i}>{d}</p>
              ))}
            </div>
          )}
          <Description
            label="About"
            value={category.shortDescription}
            className="mt-6"
          />
        </>
      }
      suffix={
        lexicalHasContent(category.description) ? (
          <section className="mt-12 max-w-3xl border-t border-border pt-8">
            <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              About {baseName}
            </h2>
            <div className="mt-3 space-y-4 text-sm leading-relaxed text-foreground">
              <LexicalRichText value={category.description} />
            </div>
          </section>
        ) : null
      }
      prefix={
        category.children.length > 0 ? (
          <nav className="mt-8">
            <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              Subcategories
            </h2>
            <ul className="mt-2 flex flex-wrap gap-2 text-sm">
              {category.children.map((c) => (
                <li key={c.id}>
                  {/* pathFilters carries the chain into child categories - empty results still beat losing it */}
                  <Link
                    href={categoryHref(c, pathFilters)}
                    className="rounded border border-border px-2.5 py-1 text-foreground hover:bg-muted"
                  >
                    {displayName(c.name, c.slug)}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
        ) : null
      }
    />
  );
}
