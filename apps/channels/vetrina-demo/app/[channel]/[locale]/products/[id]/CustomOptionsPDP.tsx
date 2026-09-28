"use client";

import { useActionState, useState } from "react";
import { useTranslations } from "next-intl";

import { addConfiguredToCart } from "@/app/lib/cart/actions";
import { displayName } from "@/app/lib/display";
import { formatPrice } from "@/app/lib/format";
import type { ProductDetail } from "@/app/lib/catalog/read";
import { ApproxPrice } from "@/app/components/ApproxPrice";
import { CompareButton } from "@/app/components/CompareButton";
import { RatingStars } from "@/app/components/RatingStars";
import { WishlistButton } from "@/app/components/WishlistButton";
import { deriveQtyBounds } from "@/app/components/qty";
import { QtySelector } from "@/app/components/QtySelector";
import { RequiredFlag } from "./RequiredFlag";
import { MissingOptionLinks } from "./MissingOptionLinks";
import { jumpToOption } from "./focus-intent";
import { AddToCartButton } from "@/app/components/product/AddToCartButton";
import { StickyBuyRow } from "@/app/components/product/StickyBuyRow";
import { AddToCartFeedback } from "@/app/components/product/AddToCartFeedback";
import { OptionControl, useConfiguredOptions } from "./ProductOptions";
import {
  AttributeList,
  Description,
  ProductGallery,
  ProductReassurance,
  optionAnchorId,
  SelectionHint,
  StockBadge,
} from "./parts";

export function CustomOptionsPDP({ product }: { product: ProductDetail }) {
  const [addState, formAction] = useActionState(addConfiguredToCart, null);
  const t = useTranslations("product.pdp");
  const tp = useTranslations("product");
  const opts = useConfiguredOptions(product.options, {
    defaultPrice: product.defaultPrice,
    effectivePrice: product.price ?? product.defaultPrice,
    productSku: product.sku,
  });
  const effectiveBase = product.price ?? product.defaultPrice;
  const qtyBounds = deriveQtyBounds(product);
  const heading = displayName(product.name, product.sku);
  // Bumped each time the shopper clicks Add-to-cart while a required option is still
  // empty
  const [attemptSeq, setAttemptSeq] = useState(0);
  const attempted = attemptSeq > 0;

  return (
    <div className="mt-6 grid grid-cols-1 gap-10 md:grid-cols-2 md:items-start">
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
        <h1 className="mt-1 text-3xl font-extrabold tracking-tight">{heading}</h1>
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
          {product.reviewAggregates &&
            product.reviewAggregates.count > 0 &&
            product.reviewAggregates.overallAverage != null && (
              <a
                href="#reviews"
                className="inline-flex items-center gap-1.5 hover:underline"
              >
                <RatingStars value={product.reviewAggregates.overallAverage} />
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

        <div className="glass mt-5 rounded-xl p-6">
        <p className="text-2xl font-bold tracking-tight">
          {formatPrice(opts.configuredTotal, product.defaultCurrencyCode)}
          <ApproxPrice cents={opts.configuredTotal} className="text-base font-normal text-muted-foreground" />
          {product.onSale && product.compareAtPrice != null && (
            <span className="ml-2 align-middle text-base font-normal text-muted-foreground line-through">
              {formatPrice(
                product.compareAtPrice + opts.optionsDelta,
                product.defaultCurrencyCode,
              )}
            </span>
          )}
        </p>
        {opts.optionsDelta !== 0 ? (
          <p className="mt-1 text-xs text-muted-foreground">
            {t("basePlusOptions", {
              base: formatPrice(effectiveBase, product.defaultCurrencyCode),
              options: formatPrice(opts.optionsDelta, product.defaultCurrencyCode),
            })}
          </p>
        ) : null}
        {opts.forcedQty != null ? (
          <p className="mt-1 text-xs text-muted-foreground">
            {t.rich("qtyLockedTotal", {
              qty: opts.forcedQty,
              total: () => (
                <span className="font-medium text-foreground">
                  {formatPrice(
                    opts.configuredTotal * (opts.forcedQty ?? 0),
                    product.defaultCurrencyCode,
                  )}
                </span>
              ),
            })}
          </p>
        ) : null}
        {opts.forcedQtyConflict ? (
          <p role="alert" className="mt-1 text-xs font-medium text-red-600 dark:text-red-400">
            {t("qtyConflict")}
          </p>
        ) : null}

        <StockBadge
          inStock={product.inStock}
          salableQuantity={product.salableQuantity}
        />

        <section className="mt-8 space-y-6">
          {opts.visibleOptions.map((opt) => (
            // Stable key (no remount) so a required text option doesn't lose focus the
            // moment it's satisfied; the flag toggles via props
            <RequiredFlag
              key={opt.id}
              active={attempted && opts.missingRequiredIds.includes(opt.id)}
              attemptSeq={attemptSeq}
              anchorId={optionAnchorId(opt.id)}
            >
              {(invalid) => (
                <OptionControl
                  option={opt}
                  currency={product.defaultCurrencyCode}
                  picked={opts.sel[opt.id] ?? new Set()}
                  textValue={opts.text[opt.id] ?? ""}
                  file={opts.files[opt.id]}
                  onToggle={(vid) => opts.toggleValue(opt, vid)}
                  onText={(v) => opts.setOptionText(opt.id, v)}
                  onFile={(e) => opts.onFileChange(opt, e)}
                  invalid={invalid}
                />
              )}
            </RequiredFlag>
          ))}
        </section>

        <StickyBuyRow className="mt-8">
          <form action={formAction} className="flex items-center gap-3">
          <input type="hidden" name="productSku" value={product.sku} />
          <input
            type="hidden"
            name="options"
            value={JSON.stringify(opts.payload)}
          />
          {opts.forcedQty != null ? (
            <input type="hidden" name="quantity" value={opts.forcedQty} />
          ) : (
            <QtySelector
              name="quantity"
              defaultValue={qtyBounds.defaultValue}
              min={qtyBounds.min}
              max={qtyBounds.max}
              step={qtyBounds.step}
              decimals={qtyBounds.decimals}
              disabled={!product.inStock}
            />
          )}
          <AddToCartButton
            stickyPrice={formatPrice(opts.configuredTotal, product.defaultCurrencyCode)}
            disabled={!product.inStock || opts.uploading}
            blocked={opts.missingRequired.length > 0 || opts.forcedQtyConflict}
            onBlockedClick={() => {
              setAttemptSeq((n) => n + 1);
              const first = opts.missingRequiredItems[0]?.id;
              if (first) jumpToOption(first);
            }}
            className="flex-1"
          >
            {opts.uploading
              ? t("uploading")
              : opts.missingRequired.length > 0
                ? t("completeRequired")
                : opts.forcedQtyConflict
                  ? t("resolveConflict")
                  : tp("addToCart")}
          </AddToCartButton>
        </form>
        </StickyBuyRow>
        <AddToCartFeedback state={addState} />

        {attempted && opts.missingRequired.length > 0 ? (
          <SelectionHint key={attemptSeq}>
            {t.rich("requiredOptions", {
              count: opts.missingRequired.length,
              links: () => <MissingOptionLinks items={opts.missingRequiredItems} />,
            })}
          </SelectionHint>
        ) : null}

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
          className="mt-8"
        />

        <AttributeList values={product.attributeValues} className="mt-8" />
      </div>
    </div>
  );
}
