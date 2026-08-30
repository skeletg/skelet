import { ViewTransition } from "react";

import { Link } from "@/app/components/LocaleLink";


import type { ProductSummary } from "@/app/lib/catalog/read";
import { mediaSrcSet, mediaThumb } from "@/app/lib/media-thumb";
import { displayName } from "@/app/lib/display";
import { productHref } from "@/app/lib/entity-href";
import { formatPrice } from "@/app/lib/format";
import { productImageTransitionName } from "@/app/lib/view-transition";
import { AddToCartForm } from "./AddToCartForm";
import { RatingStars } from "./RatingStars";
import { WishlistButton } from "./WishlistButton";

// Shared product card for every product grid - variant="compact" drops the framing for rails whose container already
// provides it No qty stepper: quantity is picked on the PDP or in the cart, the card sells the click

export function ProductCard({
  product,
  variant = "default",
  morphToDetail = false,
}: {
  product: ProductSummary;
  variant?: "default" | "compact";
  // Opt in to the shared-element morph into the PDP. Off by default because a view-transition-name
  // has to be unique per document, and the same product can sit in a rail and the grid at once -
  // only the surface that owns the page's product list should claim it
  morphToDetail?: boolean;
}) {
  const name = displayName(product.name, product.sku);
  const compact = variant === "compact";
  const isSimple = product.kind === "SIMPLE";
  // Effective price (channel listing + active specials); defaultPrice is the fallback
  // for slim payloads that predate the price field
  const sellPrice = product.price ?? product.defaultPrice;
  // A dynamically priced bundle carries no price of its own - the cart and the PDP both total its
  // picked selections instead. The card has no selections to total, so it said "$0.00" for a bundle
  // that costs $40, and quoting a price the shopper can never pay is worse than not quoting one
  const pricedBySelection = sellPrice <= 0 && !isSimple;
  const priceText = pricedBySelection
    ? "See options"
    : formatPrice(sellPrice, product.defaultCurrencyCode);
  const compareAt =
    !pricedBySelection && product.compareAtPrice && product.compareAtPrice > sellPrice
      ? formatPrice(product.compareAtPrice, product.defaultCurrencyCode)
      : null;
  // Savings badge - percentage off the compare-at price, shown from 5% up so rounding
  // noise doesn't produce a "Save 1%" sticker
  const savePct =
    !pricedBySelection && product.compareAtPrice && product.compareAtPrice > sellPrice
      ? Math.round((1 - sellPrice / product.compareAtPrice) * 100)
      : 0;
  const rating =
    product.reviewAggregates &&
    product.reviewAggregates.count > 0 &&
    product.reviewAggregates.overallAverage != null
      ? {
          value: product.reviewAggregates.overallAverage,
          count: product.reviewAggregates.count,
        }
      : null;

  const promoLabel =
    product.promotionalDisplays?.find((d) => d.labelMedia) ?? null;
  const promoPill = !promoLabel
    ? product.promotionalDisplays?.find((d) => d.pillText) ?? null
    : null;

  const cornerBadge = !product.inStock ? (
    <span className="inline-flex items-center rounded-sm border border-border bg-background/80 px-2 py-0.5 font-mono text-[0.65rem] font-medium uppercase tracking-wider backdrop-blur">
      Sold out
    </span>
  ) : savePct >= 5 ? (
    <span className="inline-flex items-center rounded-sm bg-signal px-2 py-0.5 font-mono text-[0.65rem] font-bold uppercase tracking-wider text-white">
      Save {savePct}%
    </span>
  ) : product.onSale ? (
    <span className="inline-flex items-center rounded-sm bg-signal px-2 py-0.5 font-mono text-[0.65rem] font-bold uppercase tracking-wider text-white">
      Sale
    </span>
  ) : null;

  const imageBox = (
    <div
      className={`relative aspect-square overflow-hidden bg-muted ${compact ? "rounded-lg" : ""}`}
    >
      {product.mainImage ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={mediaThumb(product.mainImage.url, compact ? 280 : 560, product.mainImage.width)}
          srcSet={mediaSrcSet(product.mainImage.url, product.mainImage.width)}
          // Cards sit in a 2-up grid on phones and 3-4 up from sm, so the slot is a
          // fraction of the viewport rather than a fixed width
          sizes="(min-width: 1024px) 280px, (min-width: 640px) 33vw, 50vw"
          alt={product.mainImage.altText || name}
          // Intrinsic size lets the browser reserve the box before the bytes land
          width={560}
          height={560}
          loading="lazy"
          decoding="async"
          className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.03]"
        />
      ) : (
        <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
          No image
        </div>
      )}
      <span className="absolute left-2 top-2">
        <WishlistButton productId={product.id} size="sm" />
      </span>
      {cornerBadge && (
        <span className="pointer-events-none absolute right-2 top-2">
          {cornerBadge}
        </span>
      )}
      {promoLabel?.labelMedia ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={mediaThumb(promoLabel.labelMedia.url, 112)}
          alt={promoLabel.labelMedia.altText || promoLabel.publicLabel}
          width={56}
          height={56}
          loading="lazy"
          decoding="async"
          className="pointer-events-none absolute bottom-2 right-2 size-14 object-contain drop-shadow-md"
        />
      ) : promoPill ? (
        <span
          className="pointer-events-none absolute bottom-2 right-2 rounded-sm px-2.5 py-0.5 text-[0.65rem] font-semibold uppercase tracking-wider shadow-sm"
          style={{
            backgroundColor: promoPill.pillBgColor || "#ef4e12",
            color: promoPill.pillTextColor || "#ffffff",
          }}
        >
          {promoPill.pillText}
        </span>
      ) : null}
    </div>
  );

  // The image is the one thing the card and the PDP genuinely share, so it is the only element that
  // morphs - the badges and wishlist button ride along inside the same box rather than flying
  // separately, which would read as debris rather than continuity
  const image = morphToDetail ? (
    <ViewTransition
      name={productImageTransitionName(product.id)}
      share="morph"
      // Without this every named card would animate on ANY transition on the page, not just the one
      // that navigates into this product
      default="none"
    >
      {imageBox}
    </ViewTransition>
  ) : (
    imageBox
  );

  const priceRow = (
    <p className="flex flex-wrap items-baseline gap-x-2 font-mono">
      <span
        className={`font-semibold tabular-nums tracking-tight ${
          pricedBySelection
            ? "text-sm text-muted-foreground"
            : compact
              ? "text-base"
              : "text-lg"
        }`}
      >
        {priceText}
      </span>
      {compareAt && (
        <span className="text-xs text-muted-foreground line-through">
          {compareAt}
        </span>
      )}
    </p>
  );

  const cta = isSimple ? (
    <AddToCartForm product={product} size="sm" hideQty />
  ) : (
    <Link
      href={productHref(product)}
      className="inline-flex h-10 w-full items-center justify-center gap-1.5 rounded-md bg-primary px-5 text-xs font-semibold text-primary-foreground transition-colors duration-200 hover:bg-primary/85"
    >
      Choose options
    </Link>
  );

  if (compact) {
    return (
      <div className="group flex h-full flex-col gap-2">
        <Link href={productHref(product)} className="flex flex-col gap-2">
          {image}
          <div className="px-0.5">
            <h3 className="line-clamp-2 text-sm font-medium leading-snug group-hover:underline">
              {name}
            </h3>
            {rating && (
              <RatingStars
                value={rating.value}
                count={rating.count}
                className="mt-1"
              />
            )}
          </div>
        </Link>
        <div className="mt-auto grid gap-2 px-0.5">
          {priceRow}
          {cta}
        </div>
      </div>
    );
  }

  return (
    <div className="glass glass-hover group h-full overflow-hidden rounded-lg">
      <div className="flex h-full flex-col">
        <Link href={productHref(product)} className="block">
          {image}
        </Link>
        <div className="flex flex-1 flex-col gap-1.5 p-5 pt-4">
          <Link href={productHref(product)} className="block">
            <h3 className="line-clamp-2 text-sm font-medium leading-snug group-hover:underline">
              {name}
            </h3>
          </Link>
          {rating && <RatingStars value={rating.value} count={rating.count} />}
          <div className="mt-auto grid gap-2 pt-2">
            {priceRow}
            <p className="flex items-center gap-1.5 font-mono text-[11px] font-medium uppercase tracking-[0.06em]">
              {product.inStock ? (
                <>
                  <span
                    aria-hidden="true"
                    className="h-1.5 w-1.5 rounded-full bg-emerald-500"
                  />
                  <span className="text-emerald-700">In stock</span>
                </>
              ) : (
                <>
                  <span
                    aria-hidden="true"
                    className="h-1.5 w-1.5 rounded-full bg-border"
                  />
                  <span className="text-muted-foreground">Out of stock</span>
                </>
              )}
            </p>
            {cta}
          </div>
        </div>
      </div>
    </div>
  );
}
