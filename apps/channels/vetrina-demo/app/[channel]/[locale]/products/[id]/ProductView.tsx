import type { Metadata } from "next";
import { Suspense } from "react";
import { getTranslations } from "next-intl/server";
import { getSiteUrl } from "@/app/lib/channel/site";
import { canonicalPathMapper, localizedAlternates } from "@/app/lib/locale/alternates";
import { notFound } from "next/navigation";

import {
  getProduct,
  getRecommendations,
  type ProductDetail,
} from "@/app/lib/catalog/read";
import { absUrl } from "@/app/lib/sitemap/read";
import { displayName } from "@/app/lib/display";
import { getChannel } from "@/app/lib/channel/read";
import { priceCurrency, toMajor } from "@/app/lib/format";
import { categoryHref, productHref } from "@/app/lib/entity-href";
import { getSignalsEnabled, getVariantOwnUrl } from "@/app/lib/config/read";
import { productJsonLd, breadcrumbJsonLd } from "@/app/lib/json-ld";
import { getProductWithReviews } from "@/app/lib/reviews/read";
import { RecordView, RecentlyViewedRail } from "@/app/components/RecentlyViewed";
import { ProductCard } from "@/app/components/ProductCard";
import { AnalyticsEvent } from "@/app/components/AnalyticsEvent";
import { Breadcrumbs } from "@/app/components/Breadcrumbs";
import { JsonLd } from "@/app/components/JsonLd";
import { lexicalSummary } from "@/app/lib/lexical-render";
import { ReviewsSection } from "./ReviewsSection";
import { ReviewStateProvider } from "./ReviewStateProvider";
import { QnaSection } from "./QnaSection";
import { BundlePDP } from "./BundlePDP";
import { ConfigurablePDP } from "./ConfigurablePDP";
import { CustomOptionsPDP } from "./CustomOptionsPDP";
import { GroupedPDP } from "./GroupedPDP";
import { SimplePDP } from "./SimplePDP";

export async function generateMetadata(
  props: {
    params: Promise<{ id: string }>;
    searchParams: Promise<Record<string, string | string[] | undefined>>;
  },
): Promise<Metadata> {
  const { id } = await props.params;
  const { product } = await getProduct(id);
  if (!product) {
    const t = await getTranslations("product");
    return { title: t("notFound") };
  }
  const title = displayName(product.metaTitle ?? product.name, product.sku);
  // Same precedence as the Product JSON-LD - an unset meta description falls back to the
  // product's own copy rather than leaving the tag empty
  const description =
    product.metaDescription ||
    lexicalSummary(product.shortDescription) ||
    lexicalSummary(product.description) ||
    undefined;
  const href = productHref(product);
  const image = product.ogImage ?? product.media?.[0];
  const base = await getSiteUrl();
  return {
    title,
    description,
    // Canonical = the product's pretty URL, so the typed /products/<id> route
    // consolidates to it instead of competing as a duplicate
    alternates: await localizedAlternates(href),
    openGraph: {
      type: "website",
      title,
      description,
      url: absUrl(href, base),
      images: image
        ? [{ url: absUrl(image.url, base), width: image.width ?? undefined, height: image.height ?? undefined, alt: image.altText ?? undefined }]
        : undefined,
    },
    twitter: {
      card: image ? "summary_large_image" : "summary",
      title,
      description,
      images: image ? [absUrl(image.url, base)] : undefined,
    },
  };
}

export default async function ProductPage(
  props: {
    params: Promise<{ id: string }>;
    searchParams: Promise<Record<string, string | string[] | undefined>>;
  },
) {
  const { id } = await props.params;
  const searchParams = (await props.searchParams) as Record<
    string,
    string | string[] | undefined
  >;
  // Reviews are fetched at the page level (not lazily inside ReviewsSection) so the
  // aggregate rating + review bodies land in the initial SSR payload. Always NEWEST -
  // re-sorting is client state inside the section, never a URL param
  const [{ product }, reviewsResult, signalsEnabled, tc] =
    await Promise.all([
      getProduct(id),
      getProductWithReviews(id),
      getSignalsEnabled(),
      getTranslations("common"),
    ]);
  if (!product) notFound();
// Per-variant URLs (Settings -> URLs & Redirects): a variant's own URL renders as its
// configurable parent with that variant preselected, staying self-canonical. Variant
// options are submitted with the parent's as one configured line
// Off, not a variant, or parent hidden here -> falls through to the simple PDP below
  const variantOwnUrl = await getVariantOwnUrl();
  const configurableParent =
    variantOwnUrl && product.kind === "SIMPLE" && product.partOf?.configurable?.parent?.id
      ? (await getProduct(product.partOf.configurable.parent.id)).product
      : null;
  const reviewsData = reviewsResult.product;

  const heading = displayName(product.name, product.sku);
  const synapseAttrs = JSON.stringify(synapseTrackedAttrs(product));
  const isConfigurable =
    product.kind === "CONFIGURABLE" && product.children.length > 0;
  const isBundle =
    product.kind === "BUNDLE" && product.bundleOptions.length > 0;
  const isGrouped =
    product.kind === "GROUPED" && product.groupedMembers.length > 0;
  // Simple products carrying custom options use the options picker instead of the plain add-to-cart form
  // (configurable/bundle/grouped have their own shells and never reach this branch)
  const hasCustomOptions =
    product.kind === "SIMPLE" && product.options.length > 0;

  // JSON-LD names the same URL the canonical tag does: the pretty rewrite path with the
  // locale prefix - never the typed /products/<id> route this page also serves
  const [canonical, base, channel] = await Promise.all([canonicalPathMapper(), getSiteUrl(), getChannel()]);
  const money = priceCurrency(product.defaultCurrencyCode, channel?.currency);
  const href = canonical(productHref(product));
  const breadcrumbItems = productBreadcrumbs(product, heading, tc("home"));
  const breadcrumbJsonLdItems = breadcrumbItems.map((it) => ({
    ...it,
    href: canonical(it.href),
  }));

  return (
    <main className="mx-auto w-full max-w-8xl px-6 md:px-10 pt-6 pb-14 max-md:pb-24">
      <JsonLd
        data={productJsonLd(
          product,
          href,
          reviewsData
            ? {
                count: reviewsData.reviewAggregates.count,
                overallAverage: reviewsData.reviewAggregates.overallAverage,
                reviews: reviewsData.reviews,
              }
            : undefined,
          base,
          money,
        )}
      />
      <JsonLd data={breadcrumbJsonLd(breadcrumbJsonLdItems, base)} />
      <AnalyticsEvent
        event="view_item"
        payload={{
          ecommerce: {
            currency: product.defaultCurrencyCode,
            value: toMajor(product.defaultPrice, money),
            items: [
              {
                item_id: product.sku, // the SKU everywhere, as the server's cart and purchase events have it
                item_name: heading,
                price: toMajor(product.defaultPrice, money),
                currency: product.defaultCurrencyCode,
              },
            ],
          },
        }}
      />
      {/* Folded into the page view so the activity timeline can render a rich product row */}
      <div
        hidden
        data-lens-entity="product"
        data-lens-name={heading}
        data-lens-image={product.mainImage?.url ?? ""}
        data-lens-price={String(product.defaultPrice)}
        data-lens-sku={product.sku}
        data-lens-attrs={synapseAttrs}
      />
      <Breadcrumbs items={breadcrumbItems} />

      {configurableParent ? (
        <ConfigurablePDP
          product={configurableParent}
          initialVariantId={product.id}
          variantOwnUrl={variantOwnUrl}
        />
      ) : isConfigurable ? (
        <ConfigurablePDP product={product} variantOwnUrl={variantOwnUrl} />
      ) : isBundle ? (
        <BundlePDP product={product} />
      ) : isGrouped ? (
        <GroupedPDP product={product} />
      ) : hasCustomOptions ? (
        <CustomOptionsPDP product={product} />
      ) : (
        <SimplePDP
          product={product}
          heading={heading}
          signalsEnabled={signalsEnabled}
        />
      )}

      <ReviewStateProvider productId={id}>
        <ReviewsSection
          productId={id}
          basePath={productHref(product)}
          searchParams={searchParams}
          reviewsData={reviewsData}
        />
      </ReviewStateProvider>

      {/* Q&A is below the fold and not SEO copy, so it streams in behind the buy box. A streamed
          boundary arrives as a hidden chunk only JavaScript swaps in, which is why the
          recommendations - internal product links crawlers should follow - are not behind one */}
      <Suspense fallback={null}>
        <QnaSection productId={id} />
      </Suspense>
      <Recommendations productId={id} />
      <RecentlyViewedRail excludeId={product.id} />
      <RecordView productId={product.id} />
    </main>
  );
}

async function Recommendations({ productId }: { productId: string }) {
  const [recs, t] = await Promise.all([
    getRecommendations(productId),
    getTranslations("product"),
  ]);
  if (recs.length === 0) return null;
  return (
    <section className="mt-16">
      <h2 className="text-lg font-semibold tracking-tight">{t("alsoLike")}</h2>
      <div className="mt-4 grid grid-cols-2 gap-6 sm:grid-cols-3 lg:grid-cols-4">
        {recs.map((p) => (
          <ProductCard key={p.id} product={p} variant="compact" />
        ))}
      </div>
    </section>
  );
}

// The attributes flagged for Synapse, flattened to slug -> display text for the lens entity
function synapseTrackedAttrs(product: ProductDetail): Record<string, string> {
  return Object.fromEntries(
    (product.attributeValues ?? [])
      .filter((av) => av.attribute.synapseTrack)
      .map((av) => {
        const v = av.selectedValues.length
          ? av.selectedValues.map((s) => s.name || s.value).join(", ")
          : typeof av.scalarValue === "string" || typeof av.scalarValue === "number"
            ? String(av.scalarValue)
            : "";
        return [av.attribute.slug, v] as const;
      })
      .filter(([, v]) => v !== ""),
  );
}

// Home -> primary category's ancestors (skipping the root) -> primary category -> product
function productBreadcrumbs(
  product: ProductDetail,
  heading: string,
  homeLabel: string,
): { name: string; href: string }[] {
  const category = product.primaryCategory;
  return [
    { name: homeLabel, href: "/" },
    ...(category
      ? [
          ...category.ancestors.slice(1).map((a) => ({
            name: displayName(a.name, a.slug),
            href: categoryHref(a),
          })),
          {
            name: displayName(category.name, category.slug),
            href: categoryHref(category),
          },
        ]
      : []),
    { name: heading, href: productHref(product) },
  ];
}
