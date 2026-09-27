"use client";

// BUNDLE PDP shell

import { useMemo, useState, useActionState } from "react";
import { Link } from "@/app/components/LocaleLink";

import { addBundleToCart } from "@/app/lib/cart/actions";
import { displayName } from "@/app/lib/display";
import { productHref } from "@/app/lib/entity-href";
import { formatPrice } from "@/app/lib/format";
import type {
  BundleOption,
  BundleSelection,
  ProductDetail,
} from "@/app/lib/catalog/read";
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
import {
  AttributeList,
  ChoiceIndicator,
  Description,
  optionCardClass,
  ProductGallery,
  ProductReassurance,
  optionAnchorId,
  SelectionHint,
  StockBadge,
} from "./parts";

type Picks = Record<string, Set<string>>; // optionId → selectionIds

function isSingleSelect(t: BundleOption["inputType"]) {
  return t === "RADIO" || t === "DROPDOWN";
}

function initialPicks(options: BundleOption[]): Picks {
  const picks: Picks = {};
  for (const opt of options) {
    const defaults = opt.selections.filter((s) => s.isDefault).map((s) => s.id);
    if (isSingleSelect(opt.inputType)) {
      picks[opt.id] = new Set(defaults.slice(0, 1));
    } else {
      picks[opt.id] = new Set(defaults);
    }
  }
  return picks;
}

export function BundlePDP({ product }: { product: ProductDetail }) {
  const [addState, formAction] = useActionState(addBundleToCart, null);
  const options = useMemo(
    () =>
      [...product.bundleOptions].sort((a, b) => a.sortOrder - b.sortOrder),
    [product.bundleOptions],
  );

  const [picks, setPicks] = useState<Picks>(() => initialPicks(options));

  const togglePick = (option: BundleOption, selectionId: string) => {
    setPicks((prev) => {
      const next: Picks = {};
      for (const k of Object.keys(prev)) {
        next[k] = new Set(prev[k]);
      }
      const bucket = next[option.id] ?? new Set<string>();
      if (isSingleSelect(option.inputType)) {
        // Tapping the already-picked one in a required slot is a no-op (you can't
        // un-pick a required radio); in an optional slot it clears the choice
        if (bucket.has(selectionId) && option.isRequired) return prev;
        bucket.clear();
        if (!prev[option.id]?.has(selectionId)) bucket.add(selectionId);
      } else {
        if (bucket.has(selectionId)) bucket.delete(selectionId);
        else bucket.add(selectionId);
      }
      next[option.id] = bucket;
      return next;
    });
  };

  const chosen: Array<{
    option: BundleOption;
    selection: BundleSelection;
  }> = [];
  for (const opt of options) {
    for (const sel of opt.selections) {
      if (picks[opt.id]?.has(sel.id)) chosen.push({ option: opt, selection: sel });
    }
  }
  const totalItemsPerBundle = chosen.reduce(
    (n, c) => n + c.selection.quantity,
    0,
  );
  // The cart prices a bundle as the sum of its picked selections - mirror that here so the headline tracks the picks
  // instead of showing the wrapper's own price (0 for dynamically priced bundles)
  const bundleTotal = chosen.reduce(
    (n, c) =>
      n +
      (c.selection.product.price ?? c.selection.product.defaultPrice) *
        c.selection.quantity,
    0,
  );
  const unsatisfiedRequired = options.filter(
    (o) => o.isRequired && (picks[o.id]?.size ?? 0) === 0,
  );
  const missingRequired = unsatisfiedRequired.map((o) => o.title);
  const missingRequiredIds = unsatisfiedRequired.map((o) => o.id);
  const missingRequiredItems = unsatisfiedRequired.map((o) => ({
    id: o.id,
    label: o.title,
  }));
  const qtyBounds = deriveQtyBounds(product);
  // Bumped each time the shopper tries to add before picking every required option
  const [attemptSeq, setAttemptSeq] = useState(0);
  const attempted = attemptSeq > 0;

  const heading = displayName(product.name, product.sku);

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
        <h1 className="mt-1 text-3xl font-extrabold tracking-tight">
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
          {formatPrice(bundleTotal, product.defaultCurrencyCode)}
          {chosen.length === 0 && product.defaultPrice > 0
            ? ` (from ${formatPrice(product.defaultPrice, product.defaultCurrencyCode)})`
            : null}
        </p>

        <StockBadge
          inStock={product.inStock}
          salableQuantity={product.salableQuantity}
        />

        <section className="mt-8 space-y-6">
          {options.map((opt) => (
            <RequiredFlag
              key={opt.id}
              active={attempted && missingRequiredIds.includes(opt.id)}
              attemptSeq={attemptSeq}
              anchorId={optionAnchorId(opt.id)}
            >
              {(invalid) => (
                <BundleOptionPanel
                  option={opt}
                  picked={picks[opt.id] ?? new Set()}
                  onToggle={(sid) => togglePick(opt, sid)}
                  invalid={invalid}
                />
              )}
            </RequiredFlag>
          ))}
        </section>

        <StickyBuyRow className="mt-6">
          <form action={formAction} className="flex items-center gap-3">
          <input type="hidden" name="productSku" value={product.sku} />
          <input
            type="hidden"
            name="selections"
            value={JSON.stringify(
              options.map((o) => ({
                optionId: o.id,
                selectionIds: Array.from(picks[o.id] ?? []),
              })),
            )}
          />
          <QtySelector
            name="quantity"
            defaultValue={qtyBounds.defaultValue}
            min={qtyBounds.min}
            max={qtyBounds.max}
            step={qtyBounds.step}
            decimals={qtyBounds.decimals}
            disabled={missingRequired.length > 0}
          />
          <AddToCartButton
            stickyPrice={formatPrice(bundleTotal, product.defaultCurrencyCode)}
            blocked={missingRequired.length > 0}
            onBlockedClick={() => {
              setAttemptSeq((n) => n + 1);
              const first = missingRequiredItems[0]?.id;
              if (first) jumpToOption(first);
            }}
            className="flex-1"
          >
            {missingRequired.length > 0
              ? "Pick required options"
              : "Add bundle to cart"}
          </AddToCartButton>
        </form>
        </StickyBuyRow>
        <AddToCartFeedback state={addState} />
        {attempted && missingRequired.length > 0 ? (
          <SelectionHint key={attemptSeq}>
            Pick required option{missingRequired.length > 1 ? "s" : ""}:{" "}
            <MissingOptionLinks items={missingRequiredItems} />
          </SelectionHint>
        ) : null}

        <section className="mt-8 rounded-xl border border-border/60 bg-muted/30 p-4 text-sm">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Your bundle
          </h2>
          {missingRequired.length > 0 ? (
            <p className="mt-2 text-muted-foreground">
              Pick one for required option
              {missingRequired.length > 1 ? "s" : ""}:{" "}
              <span className="text-foreground">
                {missingRequired.join(", ")}
              </span>
            </p>
          ) : chosen.length === 0 ? (
            <p className="mt-2 text-muted-foreground">Nothing picked yet.</p>
          ) : (
            <>
              <ul className="mt-2 space-y-1">
                {chosen.map(({ option, selection }) => (
                  <li key={selection.id} className="flex items-baseline gap-2">
                    <span className="text-muted-foreground">{option.title}:</span>
                    <span className="text-foreground">
                      {displayName(selection.product.name, selection.product.sku)}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      ×{selection.quantity}
                    </span>
                  </li>
                ))}
              </ul>
              <p className="mt-3 text-xs text-muted-foreground">
                {totalItemsPerBundle} item
                {totalItemsPerBundle === 1 ? "" : "s"} per bundle
              </p>
            </>
          )}
        </section>

        <ProductReassurance className="mt-5 border-t border-border pt-4" />
        </div>

        <Description
          label="Summary"
          value={product.shortDescription}
          className="mt-6"
        />

        <Description
          label="Description"
          value={product.description}
          className="mt-8"
        />

        <AttributeList values={product.attributeValues} className="mt-8" />
      </div>
    </div>
  );
}

function BundleOptionPanel({
  option,
  picked,
  onToggle,
  invalid = false,
}: {
  option: BundleOption;
  picked: Set<string>;
  onToggle: (selectionId: string) => void;
  invalid?: boolean;
}) {
  const sorted = [...option.selections].sort(
    (a, b) => a.sortOrder - b.sortOrder,
  );
  const multi = !isSingleSelect(option.inputType);

  return (
    <fieldset>
      <legend className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        {option.title}
        {option.isRequired ? (
          <span className="ml-1 text-red-500">*</span>
        ) : (
          <span className="ml-1 normal-case text-muted-foreground">(optional)</span>
        )}
      </legend>
      <ul className="mt-2 space-y-2">
        {sorted.map((sel) => {
          const isPicked = picked.has(sel.id);
          return (
            <li key={sel.id}>
              <label className={`${optionCardClass(isPicked, invalid)} cursor-pointer`}>
                <input
                  type={multi ? "checkbox" : "radio"}
                  name={`bundle-${option.id}`}
                  checked={isPicked}
                  onChange={() => onToggle(sel.id)}
                  className="sr-only"
                />
                <ChoiceIndicator checked={isPicked} multi={multi} />
                <div className="size-12 shrink-0 overflow-hidden rounded-lg border border-border/60 bg-muted">
                  {sel.product.mainImage ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img
                      src={sel.product.mainImage.url}
                      alt={sel.product.mainImage.altText || sel.product.sku}
                      className="h-full w-full object-cover"
                    />
                  ) : null}
                </div>
                <div className="min-w-0 flex-1">
                  <Link
                    href={productHref(sel.product)}
                    onClick={(e) => e.stopPropagation()}
                    className="text-sm font-medium hover:underline"
                  >
                    {displayName(sel.product.name, sel.product.sku)}
                  </Link>
                  <p className="text-xs text-muted-foreground">
                    {sel.product.sku} ·{" "}
                    {sel.product.inStock ? "In stock" : "Out of stock"}
                  </p>
                </div>
                <span className="shrink-0 text-sm font-medium tabular-nums">
                  {formatPrice(
                    sel.product.price ?? sel.product.defaultPrice,
                    sel.product.defaultCurrencyCode,
                  )}
                  {sel.quantity !== 1 ? (
                    <span className="ml-1 text-xs font-normal text-muted-foreground">
                      ×{sel.quantity}
                    </span>
                  ) : null}
                </span>
              </label>
            </li>
          );
        })}
      </ul>
    </fieldset>
  );
}
