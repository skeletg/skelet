import type { Metadata } from "next";
import { useTranslations } from "next-intl";
import { getTranslations } from "next-intl/server";
import { getSiteUrl } from "@skelet/storefront-next/channel/site";
import { Link } from "@/app/components/LocaleLink";
import {
  ArrowRightIcon,
  NewspaperIcon,
  SparklesIcon,
  TagIcon,
} from "lucide-react";

import { ProductCard } from "@/app/components/ProductCard";
import { PromoMosaic } from "@/app/components/PromoMosaic";
import { Rail, RailArrows, RailDots, RailProvider } from "@/app/components/Rail";
import { ValueStrip } from "@/app/components/ValueStrip";
import { CmsBlocks } from "@/app/components/cms/CmsBlocks";
import { parseDocument } from "@/app/components/cms/blocks";
import { JsonLd } from "@/app/components/JsonLd";
import { listBlogPosts } from "@skelet/storefront-next/blog/read";
import { listRailProducts } from "@skelet/storefront-next/catalog/read";
import { getHomePage } from "@skelet/storefront-next/cms/read";
import { displayName } from "@skelet/storefront-next/display";
import { categoryHref, collectionHref, productHref } from "@skelet/storefront-next/entity-href";
import { formatDate, formatPrice } from "@skelet/storefront-next/format";
import { loadHeader } from "@skelet/storefront-next/header/read";
import type { HeaderCategory } from "@skelet/storefront-next/header/read";
import { getStoreName } from "@skelet/storefront-next/channel/read";
import { cmsPageJsonLd, cmsSchemaFromSpec, parseCustomJsonLd } from "@skelet/storefront-next/json-ld";
import { canonicalPathMapper, localizedAlternates } from "@skelet/storefront-next/locale/alternates";
import { cmsPageMetadata } from "@skelet/storefront-next/seo-metadata";
import { renderJsonText } from "@skelet/storefront-next/json-text";

// A CMS homepage carries the same operator SEO fields as any CMS page (noindex/
// nofollow, canonical override, OG copy) - without this they were silently dropped
export async function generateMetadata(): Promise<Metadata> {
  const cmsHome = await getHomePage();
  // no CMS homepage → the layout's title and description stand; the canonical and
  // hreflang still have to be declared here, the layout can't know the path
  if (!cmsHome) return { alternates: await localizedAlternates("/") };
  return cmsPageMetadata(cmsHome, "/", await getSiteUrl());
}

export default async function Home() {
  const cmsHome = await getHomePage();
  const cmsDoc = parseDocument(cmsHome?.content?.body);
  if (cmsHome && cmsDoc) {
    // Same structured-data treatment as CMSPageRender in the catch-all, with the
    // homepage's canonical href ("/" locale-prefixed) - raw JSON-LD wins over the builder spec
    const [canonical, base] = await Promise.all([canonicalPathMapper(), getSiteUrl()]);
    const customJsonLd = parseCustomJsonLd(cmsHome.structuredData);
    const specJsonLd =
      customJsonLd == null && cmsHome.structuredDataSpec
        ? cmsSchemaFromSpec(cmsHome.structuredDataSpec, cmsHome, canonical("/"), base)
        : null;
    return (
      <main className="flex flex-col">
        <JsonLd data={cmsPageJsonLd(cmsHome, canonical("/"), base)} />
        {customJsonLd != null && <JsonLd data={customJsonLd} />}
        {specJsonLd != null && <JsonLd data={specJsonLd} />}
        <CmsBlocks blocks={cmsDoc.blocks} />
      </main>
    );
  }
  return <HardcodedHome />;
}

async function HardcodedHome() {
  const [brand, { posts }, { categories, collections }, t] = await Promise.all([
    getStoreName(),
    listBlogPosts({ limit: 3 }),
    loadHeader(),
    getTranslations("home"),
  ]);
  const products = await listRailProducts(categories.map((c) => c.id));
  const deals = products.filter(
    (p) => (p.compareAtPrice ?? 0) > (p.price ?? p.defaultPrice),
  );
  const heroProduct = products.find((p) => p.mainImage) ?? null;

  return (
    <main className="flex flex-col">
      <Hero brand={brand} categories={categories} product={heroProduct} />
      <ValueStrip />

      <PromoMosaic
        deals={deals}
        arrivals={products}
        category={categories[0] ?? null}
        collection={collections[0] ?? null}
      />

      {deals.length >= 3 && (
        <RailProvider>
          <Section
            eyebrow={t("limitedTime")}
            title={t("flashDeals")}
            href="/products"
            hrefLabel={t("viewAll")}
            actions={<RailArrows />}
          >
            <Rail label={t("flashDeals")}>
              {deals.map((p) => (
                <div key={p.id} className="w-56 shrink-0 snap-start sm:w-64">
                  <ProductCard product={p} variant="compact" />
                </div>
              ))}
            </Rail>
            <RailDots />
          </Section>
        </RailProvider>
      )}

      {categories.length > 0 && (
        <Section
          eyebrow={t("departments")}
          title={t("shopByCategory")}
          href="/products"
          hrefLabel={t("allProducts")}
        >
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {categories.slice(0, 8).map((c) => (
                <Link
                  key={c.id}
                  href={categoryHref(c)}
                  className="glass glass-hover group flex flex-col gap-6 rounded-lg p-5"
                >
                  <TagIcon className="h-6 w-6 text-signal" aria-hidden="true" />
                  <span className="flex items-center justify-between gap-2">
                    <span className="text-sm font-bold leading-tight group-hover:underline">
                      {displayName(c.name, c.slug)}
                    </span>
                    <ArrowRightIcon
                      className="h-4 w-4 shrink-0 text-muted-foreground transition group-hover:text-foreground"
                      aria-hidden="true"
                    />
                  </span>
                </Link>
            ))}
          </div>
        </Section>
      )}

      {products.length > 0 && (
        <RailProvider>
          <Section
            eyebrow={t("popularNow")}
            title={t("trending")}
            href="/products"
            hrefLabel={t("viewAll")}
            actions={<RailArrows />}
          >
            <Rail label={t("trending")}>
              {products.map((p) => (
                <div key={p.id} className="w-56 shrink-0 snap-start sm:w-64">
                  <ProductCard product={p} variant="compact" />
                </div>
              ))}
            </Rail>
            <RailDots />
          </Section>
        </RailProvider>
      )}

      {collections.length > 0 && (
        <Section
          eyebrow={t("curated")}
          title={t("collections")}
          href="/products"
          hrefLabel={t("shopAll")}
        >
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            {collections.slice(0, 4).map((c) => (
              <Link
                key={c.id}
                href={collectionHref(c)}
                className="glass glass-hover group flex items-center gap-4 rounded-lg p-5"
              >
                <SparklesIcon
                  className="h-5 w-5 shrink-0 text-signal"
                  aria-hidden="true"
                />
                <span className="flex-1 min-w-0">
                  <span className="block text-base font-medium group-hover:underline">
                    {displayName(c.name, c.slug)}
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {t("curatedSelection")}
                  </span>
                </span>
                <ArrowRightIcon
                  className="h-4 w-4 text-muted-foreground transition group-hover:text-foreground"
                  aria-hidden="true"
                />
              </Link>
            ))}
          </div>
        </Section>
      )}

      {posts.length > 0 && (
        <Section
          eyebrow={t("journal")}
          title={t("fromBlog")}
          href="/blog"
          hrefLabel={t("allPosts")}
        >
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {posts.map((p) => {
              const title = displayName(p.content?.title, p.slug);
              const excerpt = renderJsonText(p.content?.excerpt);
              return (
                <Link
                  key={p.id}
                  href={`/blog/${p.slug}`}
                  className="glass glass-hover group flex flex-col rounded-lg p-5"
                >
                  <NewspaperIcon
                    className="h-5 w-5 text-signal"
                    aria-hidden="true"
                  />
                  <h3 className="mt-3 text-base font-semibold group-hover:underline">
                    {title}
                  </h3>
                  {excerpt && (
                    <p className="mt-2 line-clamp-3 text-sm leading-relaxed text-muted-foreground">
                      {excerpt}
                    </p>
                  )}
                  <p className="mt-4 font-mono text-[11px] uppercase tracking-[0.06em] text-muted-foreground">
                    {p.publishedAt && (
                      <time dateTime={p.publishedAt}>
                        {formatDate(p.publishedAt)}
                      </time>
                    )}
                    {p.author && (
                      <>
                        {" · "}
                        <span>{p.author.displayName}</span>
                      </>
                    )}
                  </p>
                </Link>
              );
            })}
          </div>
        </Section>
      )}

    </main>
  );
}

function Hero({
  brand,
  categories,
  product,
}: {
  brand: string;
  categories: HeaderCategory[];
  product: Awaited<ReturnType<typeof listRailProducts>>[number] | null;
}) {
  const t = useTranslations("home.hero");
  return (
    <section className="border-b border-border bg-background">
      <div className="mx-auto grid w-full max-w-8xl items-center gap-10 px-6 py-14 md:px-10 sm:py-20 lg:grid-cols-12">
        <div className="flex flex-col items-start gap-6 lg:col-span-7">
          <p className="font-mono text-xs font-medium uppercase tracking-[0.22em] text-signal">
            {t("eyebrow", { brand })}
          </p>
          {/* Mixed-weight headline - the second clause carries the weight */}
          <h1 className="max-w-2xl text-5xl font-normal leading-[1.02] tracking-[-0.03em] sm:text-7xl">
            {t.rich("headline", {
              strong: (chunks) => <strong className="font-extrabold">{chunks}</strong>,
            })}
          </h1>
          <p className="max-w-xl text-base leading-relaxed text-muted-foreground sm:text-lg">
            {t("body", { brand })}
          </p>
          <div className="flex flex-wrap gap-3">
            <Link
              href="/products"
              className="inline-flex h-12 items-center gap-2 rounded-md bg-primary px-7 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/85"
            >
              {t("shopAll")}
            </Link>
            <Link
              href="/collections"
              className="inline-flex h-12 items-center gap-2 rounded-md border border-foreground/25 px-7 text-sm font-semibold text-foreground transition hover:border-foreground"
            >
              {t("browseCollections")}
            </Link>
          </div>
          {categories.length > 0 && (
            <div className="flex flex-wrap items-center gap-2 font-mono text-xs">
              <span className="text-muted-foreground">{t("popular")}</span>
              {categories.slice(0, 4).map((c) => (
                <Link
                  key={c.id}
                  href={categoryHref(c)}
                  className="rounded-md border border-border px-3 py-1 uppercase tracking-[0.06em] text-foreground/70 transition hover:border-foreground/50 hover:text-foreground"
                >
                  {displayName(c.name, c.slug)}
                </Link>
              ))}
            </div>
          )}
        </div>

        {/* Featured product on a grey tile - real catalog data, not artwork */}
        {product?.mainImage && (
          <Link
            href={productHref(product)}
            className="group relative hidden overflow-hidden rounded-lg bg-muted lg:col-span-5 lg:block"
          >
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={product.mainImage.url}
              alt={product.mainImage.altText || displayName(product.name, product.sku)}
              className="aspect-square h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.02]"
            />
            <span className="absolute inset-x-0 bottom-0 flex items-end justify-between gap-3 bg-gradient-to-t from-black/60 to-transparent p-5 text-white">
              <span className="min-w-0">
                <span className="block font-mono text-[11px] uppercase tracking-[0.14em] text-white/70">
                  {t("featured")}
                </span>
                <span className="block truncate text-base font-semibold">
                  {displayName(product.name, product.sku)}
                </span>
              </span>
              <span className="shrink-0 font-mono text-base font-semibold tabular-nums">
                {formatPrice(product.price ?? product.defaultPrice, product.defaultCurrencyCode)}
              </span>
            </span>
          </Link>
        )}
      </div>
    </section>
  );
}

// The header's right side belongs to whatever drives the content - a rail puts its arrows there.
// The "view all" is a real button under the content instead, where you land after reading it
function Section({
  eyebrow,
  title,
  href,
  hrefLabel,
  actions,
  children,
}: {
  eyebrow: string;
  title: string;
  href: string;
  hrefLabel: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-8xl px-6 md:px-10 py-14">
        <div className="mb-6 flex items-end justify-between gap-4">
          <div>
            <p className="font-mono text-xs font-medium uppercase tracking-[0.18em] text-signal">
              {eyebrow}
            </p>
            <h2 className="mt-1 text-3xl font-extrabold tracking-tight">
              {title}
            </h2>
          </div>
          {actions}
        </div>
        {children}
        <div className="mt-8 flex justify-center">
          <Link
            href={href}
            className="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-foreground/25 px-5 text-sm font-semibold transition hover:border-foreground"
          >
            {hrefLabel}
            <ArrowRightIcon className="h-4 w-4" aria-hidden="true" />
          </Link>
        </div>
      </div>
    </section>
  );
}

