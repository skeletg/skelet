// One cached HTML document per path, on a timer and busted on demand by the backend's revalidate
// queue - force-static neutralises cookies/searchParams so every visitor can share the same render
export const dynamic = "force-static";
export const revalidate = 300;
export const dynamicParams = true;

// Catch-all route: every path the storefront doesn't have a typed page for lands here

import { getSiteUrl } from "@skelet/storefront-next/channel/site";
import type { Metadata } from "next";
import { canonicalPathMapper, localizedAlternates } from "@skelet/storefront-next/locale/alternates";
import type { ReactNode } from "react";
import { Link } from "@/app/components/LocaleLink";
import { notFound, permanentRedirect, redirect } from "next/navigation";
import BlogPostPage, { generateMetadata as blogPostMetadata } from "@/app/[channel]/[locale]/blog/[slug]/page";
import CategoryPage, { generateMetadata as categoryMetadata } from "@/app/[channel]/[locale]/categories/[id]/[[...filter]]/CategoryView";
import CollectionPage, { generateMetadata as collectionMetadata } from "@/app/[channel]/[locale]/collections/[id]/[[...filter]]/CollectionView";
import ProductPage, { generateMetadata as productMetadata } from "@/app/[channel]/[locale]/products/[id]/ProductView";
import { blogTaxonomyHrefById, getBlogPostById } from "@skelet/storefront-next/blog/read";
import { getCategory, getCollection } from "@skelet/storefront-next/catalog/read";
import { categoryHref, collectionHref } from "@skelet/storefront-core/entity-href";
import { getCMSPage, getHomePage, type CMSPage } from "@skelet/storefront-next/cms/read";
import { cmsPageMetadata } from "@skelet/storefront-next/seo-metadata";
import { displayName } from "@skelet/storefront-core/display";
import { PAGE_MARKER, splitSlug } from "@skelet/storefront-core/filter-path";
import { renderJsonText } from "@skelet/storefront-core/json-text";
import { getTranslations, setRequestLocale } from "next-intl/server";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";

import { resolveRoute } from "@skelet/storefront-next/url-resolver/read";
import { CmsBlocks } from "@/app/components/cms/CmsBlocks";
import { parseDocument } from "@/app/components/cms/blocks";
import { JsonLd } from "@/app/components/JsonLd";
import { CmsVariantPicker } from "@/app/components/cms/CmsVariantPicker";
import { cmsPageJsonLd, breadcrumbJsonLd, cmsSchemaFromSpec, parseCustomJsonLd } from "@skelet/storefront-core/json-ld";

type DelegatePage = (props: {
  params: Promise<Record<string, string | string[] | undefined>>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) => Promise<ReactNode>;

// Same brand-suppressing cast for the typed pages' generateMetadata exports, so pretty URLs carry the SAME rich
// metadata (title, description, OG/Twitter, canonical) as the typed routes instead of the lean slug-only fallback
type DelegateMetadata = (props: {
  params: Promise<Record<string, string | string[] | undefined>>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) => Promise<Metadata>;

// Re-serialise a parsed filter chain into the `[[...filter]]` segment array shape the
// delegated pages expect - page last, and only past page 1, matching the URL grammar
function buildFilterParam(
  filters: ReturnType<typeof splitSlug>["filters"],
  sortSlug: string,
  page = 1,
): string[] | undefined {
  const fSegs = filters.flatMap((f) => ["f", f.slug, ...f.values.flatMap((v) => ["v", v])]);
  const sSegs = sortSlug ? ["s", sortSlug] : [];
  const pSegs = page > 1 ? [PAGE_MARKER, String(page)] : [];
  const all = [...fSegs, ...sSegs, ...pSegs];
  return all.length > 0 ? all : undefined;
}

// Id-based entity URLs (/categories/<uuid>, /collections/<uuid>) have no route of their own - there are no typed [id]
// routes, so these paths fall through to this catch-all
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

async function legacyEntityIdHref(slug: string[]): Promise<string | null> {
  if (slug.length < 2 || !UUID_RE.test(slug[1])) return null;
  const id = slug[1];
  if (slug[0] === "categories") {
    const { category } = await getCategory(id);
    return category ? categoryHref(category) : null;
  }
  if (slug[0] === "collections") {
    const { collection } = await getCollection(id);
    return collection ? collectionHref(collection) : null;
  }
  return null;
}

type CatchAllParams = {
  locale: string;
  slug: string[];
};

export async function generateMetadata({
  params,
  searchParams,
}: {
  params: Promise<CatchAllParams>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}): Promise<Metadata> {
  const { locale, slug } = await params;
  setRequestLocale(locale);
  const { entityPath, filters, sortSlug, page } = splitSlug(slug);
  const r = await resolveRoute(entityPath);
  if (!r) {
    const t = await getTranslations({ locale, namespace: "errors" });
    return { title: t("notFoundShort") };
  }
  // CMS pages own no typed page of their own, so the catch-all is where their full SEO metadata is emitted: title/description from meta,
  // plus Open Graph + Twitter Card (absolute og:image - there's no metadataBase) and a noindex robots tag when flagged
  if (r.kind === "CMS_PAGE") {
    const page = await getCMSPage(r.entityId);
    if (page) return cmsPageMetadata(page, "/" + entityPath, await getSiteUrl());
  }
  if (!r.redirectTo) {
    const filterParam = buildFilterParam(filters, sortSlug, page);
    const empty = Promise.resolve({});
    switch (r.kind) {
      case "PRODUCT":
        return (productMetadata as DelegateMetadata)({
          params: Promise.resolve({ id: r.entityId }),
          searchParams: empty,
        });
      case "CATEGORY":
        return (categoryMetadata as DelegateMetadata)({
          params: Promise.resolve({ id: r.entityId, filter: filterParam }),
          searchParams, // paged canonicals read ?page
        });
      case "COLLECTION":
        return (collectionMetadata as DelegateMetadata)({
          params: Promise.resolve({ id: r.entityId, filter: filterParam }),
          searchParams,
        });
      case "BLOG_POST": {
        const post = await getBlogPostById(r.entityId);
        if (post) {
          return (blogPostMetadata as DelegateMetadata)({
            params: Promise.resolve({ slug: post.slug }),
            searchParams: empty,
          });
        }
      }
    }
  }
  // Lean fallback for kinds without a typed page (blog taxonomy, plugin entities) and
  // redirect rows
  return { title: entityPath, alternates: await localizedAlternates("/" + entityPath) };
}

export default async function CatchAllPage({
  params,
  searchParams,
}: {
  params: Promise<CatchAllParams>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}): Promise<ReactNode> {
  const { locale, slug } = await params;
  // Static rendering gives each page its own scope, so the layout's setRequestLocale does not
  // reach here - without this every prefixed url would fetch content in the default language
  setRequestLocale(locale);
  // Id-based /categories/<uuid> & /collections/<uuid> URLs → 301 to the
  // canonical pretty URL (their typed routes were removed)
  const legacyHref = await legacyEntityIdHref(slug);
  if (legacyHref) permanentRedirect(await withLocalePrefix(legacyHref, locale));
  const { entityPath, filters, sortSlug, page } = splitSlug(slug);
  const r = await resolveRoute(entityPath);
  if (!r) notFound();
  // Filter + sort chain re-serialised as the `[[...filter]]` array shape the delegated
  // pages expect
  const filterParam = buildFilterParam(filters, sortSlug, page);

  const forwardedSearchParams = await searchParams;
  const prettyBasePath = "/" + entityPath;

  if (r.redirectTo) {
    const target = r.redirectTo.startsWith("/") ? r.redirectTo : "/" + r.redirectTo;
    // Prefixed: this fires mid-browse on every canonicalising url, and an unprefixed target
    // is what threw a German visitor back into English
    const localized = await withLocalePrefix(target, locale);
    if (r.redirectStatusCode === 301) permanentRedirect(localized);
    redirect(localized);
  }

  switch (r.kind) {
    case "PRODUCT":
      return (ProductPage as DelegatePage)({
        params: Promise.resolve({ id: r.entityId }),
        searchParams: Promise.resolve(forwardedSearchParams),
      });
    case "CATEGORY":
      return (CategoryPage as DelegatePage)({
        params: Promise.resolve({
          id: r.entityId,
          filter: filterParam,
          basePath: prettyBasePath,
        }),
        searchParams: Promise.resolve(forwardedSearchParams),
      });
    case "COLLECTION":
      return (CollectionPage as DelegatePage)({
        params: Promise.resolve({
          id: r.entityId,
          filter: filterParam,
          basePath: prettyBasePath,
        }),
        searchParams: Promise.resolve(forwardedSearchParams),
      });
    case "BLOG_POST": {
      // The typed blog route uses slug, not id
      const post = await getBlogPostById(r.entityId);
      if (!post) notFound();
      return (BlogPostPage as DelegatePage)({
        params: Promise.resolve({ slug: post.slug }),
        searchParams: Promise.resolve({}),
      });
    }
    case "CMS_PAGE": {
      const page = await getCMSPage(r.entityId);
      if (!page || !page.isPublished) notFound();
      // Dedup: if this page is the active channel's homepage (served at "/"), collapse its own url_path to the single
      // canonical homepage URL so the same content isn't crawlable at two paths
      const home = await getHomePage();
      if (home && home.id === page.id) permanentRedirect(await withLocalePrefix("/", locale));
      if (page.experimentKey) {
        return (
          <CmsVariantPicker
            experiment={page.experimentKey}
            control={<CMSPageRender page={page} body={page.content?.body} />}
            variants={page.variants.flatMap((v) =>
              v.body
                ? [{ key: v.variantKey, node: <CMSPageRender page={page} body={v.body} /> }]
                : [],
            )}
          />
        );
      }
      return <CMSPageRender page={page} />;
    }
    // The taxonomy archives are typed routes under /blog/*, so their canonical paths never reach the catch-all - this
    // only fires for an alias rewrite row pointing at one from somewhere else
    case "BLOG_CATEGORY":
    case "BLOG_TAG":
    case "BLOG_AUTHOR": {
      const href = await blogTaxonomyHrefById(r.kind, r.entityId);
      if (href) permanentRedirect(await withLocalePrefix(href, locale));
      notFound();
    }
    // PLUGIN-kind routes (and any future unhandled kind) resolve on the backend - `r.pluginKind` carries the
    // entity_type slug - but the storefront has no renderer for them yet
    default:
      notFound();
  }
}

// CMS pages have no typed route, so this is the only surface that renders them A block Document goes to CmsBlocks
// full-bleed; anything else is freeform and falls back to plain text in a content column
async function CMSPageRender({ page, body: bodyOverride }: { page: CMSPage; body?: unknown }) {
  const title = displayName(page.content?.title, page.slug);
  const effectiveBody = bodyOverride ?? page.content?.body;
  // Canonical href = the page's pretty path, locale-prefixed like the canonical tag
  const [canonical, base, t, tc] = await Promise.all([
    canonicalPathMapper(),
    getSiteUrl(),
    getTranslations("cms"),
    getTranslations("common"),
  ]);
  const href = canonical("/" + page.urlPath);
  // Structured data: a WebPage node + a Home→page breadcrumb + the operator's own JSON-LD -
  // noindex pages still emit it (noindex governs the index listing, not on-page markup,
  // and the page isn't in the sitemap regardless)
  // Raw JSON-LD wins over the builder spec when both are set, matching the resolver's note
  const customJsonLd = parseCustomJsonLd(page.structuredData);
  const specJsonLd =
    customJsonLd == null && page.structuredDataSpec
      ? cmsSchemaFromSpec(page.structuredDataSpec, page, href, base)
      : null;
  const seo = (
    <>
      <JsonLd data={cmsPageJsonLd(page, href, base)} />
      <JsonLd
        data={breadcrumbJsonLd([
          { name: tc("home"), href: canonical("/") },
          { name: title, href },
        ], base)}
      />
      {customJsonLd != null && <JsonLd data={customJsonLd} />}
      {specJsonLd != null && <JsonLd data={specJsonLd} />}
    </>
  );
  const doc = parseDocument(effectiveBody);
  if (doc) {
    return (
      <main className="flex flex-col">
        {seo}
        <CmsBlocks blocks={doc.blocks} />
      </main>
    );
  }
  const body = renderJsonText(effectiveBody);
  return (
    <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-12">
      {seo}
      <h1 className="text-3xl font-extrabold tracking-tight">{title}</h1>
      {page.publishedAt && (
        <p className="mt-2 text-xs uppercase tracking-wider text-muted-foreground">
          {new Date(page.publishedAt).toDateString()}
        </p>
      )}
      {body ? (
        <div className="mt-8 whitespace-pre-wrap text-base leading-relaxed text-foreground">
          {body}
        </div>
      ) : (
        <p className="mt-8 text-sm text-muted-foreground">
          {t("noContent")}
        </p>
      )}
      <p className="mt-12 text-xs text-muted-foreground">
        <Link href="/" className="underline">
          {t("backHome")}
        </Link>
      </p>
    </main>
  );
}
