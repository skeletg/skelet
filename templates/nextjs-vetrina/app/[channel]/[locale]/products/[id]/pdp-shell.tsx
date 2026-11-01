// The frame every product kind renders into - title, sticky gallery, meta row, buy box. Like parts.tsx it holds
// no state and only calls useTranslations, so the server-rendered simple PDP and the client shells both import it

import type { ComponentProps, ReactNode } from "react";
import { useTranslations } from "next-intl";

import type { ProductDetail } from "@skelet/storefront-next/catalog/read";
import { displayName } from "@skelet/storefront-core/display";
import { formatPrice } from "@skelet/storefront-core/format";
import { CompareButton } from "@/app/components/CompareButton";
import { QtySelector } from "@/app/components/QtySelector";
import { RatingStars } from "@/app/components/RatingStars";
import { WishlistButton } from "@/app/components/WishlistButton";
import type { QtyBounds } from "@/app/components/qty";
import { ProductGallery, ProductReassurance } from "./parts";

type GalleryProps = ComponentProps<typeof ProductGallery>;

export function PdpShell({
  product,
  heading,
  media = product.media,
  fallback = product.mainImage,
  largeHeading = false,
  children,
}: {
  product: ProductDetail;
  heading: string;
  // A configurable shell swaps in the matched variant's gallery
  media?: GalleryProps["media"];
  fallback?: GalleryProps["fallback"];
  largeHeading?: boolean;
  children: ReactNode;
}) {
  return (
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
        <h1
          className={
            largeHeading
              ? "mt-1 text-3xl font-extrabold tracking-tight sm:text-4xl"
              : "mt-1 text-3xl font-extrabold tracking-tight"
          }
        >
          {heading}
        </h1>
      </div>
      <div className="md:col-start-1 md:row-start-1 md:row-span-2 md:sticky md:top-28 md:self-start">
        <ProductGallery
          media={media}
          fallback={fallback}
          alt={heading}
          morphFromProductId={product.id}
        />
      </div>

      <div className="md:col-start-2 md:row-start-2">{children}</div>
    </div>
  );
}

// Star row (links to the reviews below), SKU, wishlist and compare
export function PdpMetaRow({
  product,
  sku = product.sku,
}: {
  product: ProductDetail;
  sku?: string;
}) {
  const reviews = product.reviewAggregates;
  return (
    <div className="mt-2 flex flex-wrap items-center gap-3">
      {reviews && reviews.count > 0 && reviews.overallAverage != null && (
        <a
          href="#reviews"
          className="inline-flex items-center gap-1.5 hover:underline"
        >
          <RatingStars value={reviews.overallAverage} />
          <span className="text-sm text-muted-foreground">
            {reviews.overallAverage.toFixed(1)} ({reviews.count})
          </span>
        </a>
      )}
      <p className="text-sm text-muted-foreground">SKU {sku}</p>
      <WishlistButton productId={product.id} size="sm" />
      <CompareButton productId={product.id} />
    </div>
  );
}

export function PdpBuyBox({ children }: { children: ReactNode }) {
  return (
    <div className="glass mt-5 rounded-xl p-6">
      {children}
      <ProductReassurance className="mt-5 border-t border-border pt-4" />
    </div>
  );
}

export function PdpTaxonomy({ product }: { product: ProductDetail }) {
  const tp = useTranslations("product");
  if (product.categories.length === 0 && product.collections.length === 0) {
    return null;
  }
  return (
    <section className="mt-8 text-sm">
      {product.primaryCategory && (
        <p className="text-muted-foreground">
          {tp.rich("primaryCategory", {
            value: () => (
              <span className="text-foreground">
                {displayName(
                  product.primaryCategory!.name,
                  product.primaryCategory!.slug,
                )}
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
  );
}

// The buy form's quantity. A picked option can force the line quantity - then it's submitted as-is and the
// stepper is hidden
export function PdpQty({
  bounds,
  forcedQty = null,
  disabled,
}: {
  bounds: QtyBounds;
  forcedQty?: number | null;
  disabled: boolean;
}) {
  if (forcedQty != null) {
    return <input type="hidden" name="quantity" value={forcedQty} />;
  }
  return (
    <QtySelector
      name="quantity"
      defaultValue={bounds.defaultValue}
      min={bounds.min}
      max={bounds.max}
      step={bounds.step}
      decimals={bounds.decimals}
      disabled={disabled}
    />
  );
}

// Add-to-cart label while custom options are in play, most urgent blocker first
export function OptionsAddLabel({
  uploading,
  missingRequired,
  forcedQtyConflict,
  unavailable = false,
}: {
  uploading: boolean;
  missingRequired: boolean;
  forcedQtyConflict: boolean;
  unavailable?: boolean;
}) {
  const t = useTranslations("product.pdp");
  const tp = useTranslations("product");
  if (uploading) return t("uploading");
  if (unavailable) return t("ruleBlocked");
  if (missingRequired) return t("completeRequired");
  if (forcedQtyConflict) return t("resolveConflict");
  return tp("addToCart");
}

// What the product's option rules do to the price for these picks, one row each as the cart line will list them
export function RuleAdjustments({
  adjustments,
  unavailable,
  currency,
}: {
  adjustments: { id: string; label: string; amount: number }[];
  unavailable: boolean;
  currency: string;
}) {
  const t = useTranslations("product.pdp");
  if (adjustments.length === 0 && !unavailable) return null;
  return (
    <div className="mt-1 space-y-0.5 text-xs">
      {adjustments.map((a) => (
        <p key={a.id} className="text-muted-foreground">
          {a.label || t("ruleAdjustment")}:{" "}
          <span className="font-medium text-foreground">
            {a.amount > 0 ? "+" : ""}
            {formatPrice(a.amount, currency)}
          </span>
        </p>
      ))}
      {unavailable ? (
        <p role="alert" className="font-medium text-red-600 dark:text-red-400">
          {t("ruleUnavailable")}
        </p>
      ) : null}
    </div>
  );
}
