import type { Metadata } from "next";
import { Suspense } from "react";
import { getTranslations } from "next-intl/server";
import { getSiteUrl } from "@/app/lib/channel/site";
import { canonicalPathMapper, localizedAlternates } from "@/app/lib/locale/alternates";
import { notFound } from "next/navigation";

import { formatPrice } from "@/app/lib/cart/read";
import { ApproxPrice } from "@/app/components/ApproxPrice";
import { getProduct, getRecommendations } from "@/app/lib/catalog/read";
import { absUrl } from "@/app/lib/sitemap/read";
import { displayName } from "@/app/lib/display";
import { categoryHref, productHref } from "@/app/lib/entity-href";
import { getSignalsEnabled, getVariantOwnUrl } from "@/app/lib/config/read";
import { ExperimentGate } from "@/app/components/ExperimentGate";
import { productJsonLd, breadcrumbJsonLd } from "@/app/lib/json-ld";
import { getProductWithReviews } from "@/app/lib/reviews/read";
import { AddToCartForm } from "@/app/components/AddToCartForm";
import { RatingStars } from "@/app/components/RatingStars";
import { WishlistButton } from "@/app/components/WishlistButton";
import { CompareButton } from "@/app/components/CompareButton";
import { RecordView, RecentlyViewedRail } from "@/app/components/RecentlyViewed";
import { ProductCard } from "@/app/components/ProductCard";
import {
  ProductGridSkeleton,
  Skeleton,
  SkeletonRegion,
} from "@/app/components/Skeleton";
import { BackInStockForm } from "@/app/components/BackInStockForm";
import { AnalyticsEvent } from "@/app/components/AnalyticsEvent";
import { Breadcrumbs } from "@/app/components/Breadcrumbs";
import { JsonLd } from "@/app/components/JsonLd";
import { ReviewsSection } from "./ReviewsSection";
import { ReviewStateProvider } from "./ReviewStateProvider";
import { QnaSection } from "./QnaSection";
import { BundlePDP } from "./BundlePDP";
import { ConfigurablePDP } from "./ConfigurablePDP";
import { CustomOptionsPDP } from "./CustomOptionsPDP";
import { GroupedPDP } from "./GroupedPDP";
import {
  AttributeList,
  Description,
  ProductGallery,
  ProductReassurance,
  StockBadge,
} from "./parts";

const PDP_LAYOUT_EXPERIMENT = "pdp_layout";

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
  const description = product.metaDescription ?? undefined;
  const href = productHref(product);
  const image = product.media?.[0];
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
  const [{ product }, reviewsResult, signalsEnabled, tp, tc] =
    await Promise.all([
      getProduct(id),
      getProductWithReviews(id),
      getSignalsEnabled(),
      getTranslations("product"),
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
  const synapseAttrs = JSON.stringify(
    Object.fromEntries(
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
    ),
  );
  const isConfigurable =
    product.kind === "CONFIGURABLE" && product.children.length > 0;
  const isBundle =
    product.kind === "BUNDLE" && product.bundleOptions.length > 0;
  const isGrouped =
    product.kind === "GROUPED" && product.groupedMembers.length > 0;
  // Simple products carrying custom options use the options picker instead of the plain add-to-cart form
  // (configurable/bundle/grouped have their own shells above and never reach this branch)
  const hasCustomOptions =
    product.kind === "SIMPLE" && product.options.length > 0;

  // JSON-LD names the same URL the canonical tag does: the pretty rewrite path with the
  // locale prefix - never the typed /products/<id> route this page also serves
  const [canonical, base] = await Promise.all([canonicalPathMapper(), getSiteUrl()]);
  const href = canonical(productHref(product));
  const breadcrumbItems = [
    { name: tc("home"), href: "/" },
    ...(product.primaryCategory
      ? [
          ...product.primaryCategory.ancestors.slice(1).map((a) => ({
            name: displayName(a.name, a.slug),
            href: categoryHref(a),
          })),
          {
            name:
              displayName(product.primaryCategory.name, product.primaryCategory.slug),
            href: categoryHref(product.primaryCategory),
          },
        ]
      : []),
    { name: heading, href: productHref(product) },
  ];
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
        )}
      />
      <JsonLd data={breadcrumbJsonLd(breadcrumbJsonLdItems, base)} />
      <AnalyticsEvent
        event="view_item"
        payload={{
          ecommerce: {
            currency: product.defaultCurrencyCode,
            value: product.defaultPrice / 100,
            items: [
              {
                item_id: product.id,
                item_name: heading,
                item_sku: product.sku,
                price: product.defaultPrice / 100,
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
        <div className="mt-6 grid grid-cols-1 gap-10 md:grid-cols-2 md:items-start">
          {/* items-start on the grid is what lets the gallery's sticky actually stick */}
          {/* Source order gives phones the name first; md+ placement restores the columns */}
          <div className="md:col-start-2 md:row-start-1">
            {product.primaryCategory && (
              <p className="font-mono text-xs font-medium uppercase tracking-[0.18em] text-signal">
                {displayName(
                  product.primaryCategory.name,
                  product.primaryCategory.slug,
                )}
              </p>
            )}
            <h1 className="mt-1 text-3xl font-extrabold tracking-tight sm:text-4xl">
              {heading}
            </h1>
          </div>
          <div className="md:col-start-1 md:row-start-1 md:row-span-2 md:sticky md:top-28 md:self-start">
            <ProductGallery
              media={product.media}
              fallback={product.mainImage}
              alt={heading}
              morphFromProductId={product.id}
            />
          </div>
          <div className="md:col-start-2 md:row-start-2">
            <div className="mt-2 flex flex-wrap items-center gap-3">
              {/* Star row links straight to the reviews section below */}
              {product.reviewAggregates &&
                product.reviewAggregates.count > 0 &&
                product.reviewAggregates.overallAverage != null && (
                  <a
                    href="#reviews"
                    className="inline-flex items-center gap-1.5 hover:underline"
                  >
                    <RatingStars
                      value={product.reviewAggregates.overallAverage}
                    />
                    <span className="text-sm text-muted-foreground">
                      {product.reviewAggregates.overallAverage.toFixed(1)} (
                      {product.reviewAggregates.count})
                    </span>
                  </a>
                )}
              <p className="text-sm text-muted-foreground">SKU {product.sku}</p>
              <WishlistButton productId={product.id} size="sm" />
              <CompareButton productId={product.id} />
            </div>

            {/* Rules whose product-level conditions match, priority asc - cart-level ones show in cart */}
            {product.promotionalDisplays?.length
              ? product.promotionalDisplays
                  .filter((d) => d.bannerMedia)
                  .map((d) => (
                    <div
                      key={d.ruleId}
                      className="mt-4 overflow-hidden rounded-lg"
                    >
                      {/* eslint-disable-next-line @next/next/no-img-element */}
                      <img
                        src={d.bannerMedia!.url}
                        alt={d.bannerMedia!.altText || d.publicLabel}
                        className="w-full object-cover"
                      />
                    </div>
                  ))
              : null}

            {/* For rules with no banner image, so a text-only promo still reads near the price */}
            {product.promotionalDisplays?.some(
              (d) => d.pillText && !d.bannerMedia,
            ) ? (
              <div className="mt-4 flex flex-wrap gap-2">
                {product.promotionalDisplays
                  .filter((d) => d.pillText && !d.bannerMedia)
                  .map((d) => (
                    <span
                      key={d.ruleId}
                      className="inline-flex items-center rounded-sm px-3 py-1 text-xs font-semibold uppercase tracking-wider"
                      style={{
                        backgroundColor: d.pillBgColor || "#dc2626",
                        color: d.pillTextColor || "#ffffff",
                      }}
                    >
                      {d.pillText}
                    </span>
                  ))}
              </div>
            ) : null}

            <div className="glass mt-5 rounded-xl p-6">
            <p className="font-mono text-3xl font-bold tracking-tight tabular-nums">
              {formatPrice(product.price ?? product.defaultPrice, product.defaultCurrencyCode)}
              <ApproxPrice cents={product.price ?? product.defaultPrice} className="text-base font-normal text-muted-foreground" />
              {product.compareAtPrice &&
              product.compareAtPrice > (product.price ?? product.defaultPrice) ? (
                <span className="ml-2 align-middle text-lg font-normal text-muted-foreground line-through">
                  {formatPrice(product.compareAtPrice, product.defaultCurrencyCode)}
                </span>
              ) : null}
              {product.onSale ? (
                <span className="ml-2 inline-block -translate-y-1 rounded-sm bg-signal px-2.5 py-0.5 align-middle font-mono text-xs font-semibold uppercase tracking-wider text-white">
                  {tp("sale")}
                </span>
              ) : null}
              {product.subscriptionTerms ? (
                <span className="ml-1 align-middle text-base font-normal text-muted-foreground">
                  {" / "}
                  {product.subscriptionTerms.intervalCount === 1
                    ? product.subscriptionTerms.intervalUnit.toLowerCase()
                    : `${product.subscriptionTerms.intervalCount} ${product.subscriptionTerms.intervalUnit.toLowerCase()}s`}
                </span>
              ) : null}
            </p>
            {product.subscriptionTerms && product.subscriptionTerms.trialDays > 0 ? (
              <p className="mt-1 text-sm text-muted-foreground">
                {product.subscriptionTerms.trialDays}-day free trial, then recurring billing.
              </p>
            ) : null}

            {product.tierPrices && product.tierPrices.length > 0 ? (
              <div className="mt-3 w-fit rounded-lg border border-border/60 px-4 py-3">
                <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  {tp("volumePricing")}
                </p>
                <ul className="mt-1.5 grid gap-0.5 text-sm">
                  {[...product.tierPrices]
                    .sort((a, b) => a.minQty - b.minQty)
                    .map((t) => (
                      <li key={t.minQty} className="tabular-nums">
                        {tp.rich("tierPrice", {
                          min: t.minQty,
                          price: () => (
                            <span className="font-semibold">
                              {formatPrice(t.price, product.defaultCurrencyCode)}
                            </span>
                          ),
                        })}
                      </li>
                    ))}
                </ul>
              </div>
            ) : null}

            <StockBadge
              inStock={product.inStock}
              salableQuantity={product.salableQuantity}
            />

            {!product.inStock && product.backorderable ? (
              <p className="mt-2 text-sm text-amber-700 dark:text-amber-400">
                {tp("backorder")}
              </p>
            ) : null}

            {signalsEnabled && !product.inStock && !product.backorderable ? (
              <BackInStockForm
                productId={product.id}
                sku={product.sku}
                name={heading}
                image={product.mainImage?.url ?? ""}
              />
            ) : null}

            {/* Prism A/B "pdp_layout" - layout tokens are set as each variation's value in the dashboard */}
            <div className="mt-5">
              <AddToCartForm
                product={product}
                size="md"
                mobileSticky
                stickyPrice={formatPrice(product.price ?? product.defaultPrice, product.defaultCurrencyCode)}
              />
              <ExperimentGate experiment={PDP_LAYOUT_EXPERIMENT} value="prominent_cta">
                <p className="mt-2 text-center text-xs text-muted-foreground">
                  {tp("ctaNote")}
                </p>
              </ExperimentGate>
            </div>

            <ProductReassurance className="mt-5 border-t border-border pt-4" />
            </div>

            <Description
              label={tp("summary")}
              value={product.shortDescription}
              className="mt-6"
            />
            <Description
              label={tp("description")}
              value={product.description}
              className="mt-6"
            />

            <AttributeList
              values={product.attributeValues}
              className="mt-8"
            />

            {(product.categories.length > 0 ||
              product.collections.length > 0) && (
              <section className="mt-8 text-sm">
                {product.primaryCategory && (
                  <p className="text-muted-foreground">
                    {tp.rich("primaryCategory", {
                      value: () => (
                        <span className="text-foreground">
                          {product.primaryCategory!.name ??
                            product.primaryCategory!.slug}
                        </span>
                      ),
                    })}
                  </p>
                )}
                {product.collections.length > 0 && (
                  <p className="mt-1 text-muted-foreground">
                    {tp.rich("collections", {
                      value: () => (
                        <span className="text-foreground">
                          {product.collections
                            .map((c) => displayName(c.name, c.slug))
                            .join(", ")}
                        </span>
                      ),
                    })}
                  </p>
                )}
              </section>
            )}
          </div>
        </div>
      )}

      <ReviewStateProvider productId={id}>
        <ReviewsSection
          productId={id}
          basePath={productHref(product)}
          searchParams={searchParams}
          reviewsData={reviewsData}
        />
      </ReviewStateProvider>

      {/* Both below the fold and both their own backend round trip - streamed in behind the rest of the page
          rather than holding the buy box back until they answer */}
      <Suspense fallback={null}>
        <QnaSection productId={id} />
      </Suspense>

      <Suspense
        fallback={
          <SkeletonRegion className="mt-16">
            <Skeleton className="h-6 w-48" />
            <div className="mt-4">
              <ProductGridSkeleton count={4} />
            </div>
          </SkeletonRegion>
        }
      >
        <Recommendations productId={id} />
      </Suspense>
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
