"use client";

import {
  startTransition,
  useActionState,
  useEffect,
  useMemo,
  useState,
  ViewTransition,
} from "react";

import { addConfiguredToCart, addSimpleToCart } from "@/app/lib/cart/actions";
import { displayName } from "@/app/lib/display";
import { productHref } from "@/app/lib/entity-href";
import { formatPrice } from "@/app/lib/format";
import type { ProductDetail } from "@/app/lib/catalog/read";
import { deriveQtyBounds } from "@/app/components/qty";
import { QtySelector } from "@/app/components/QtySelector";
import { CompareButton } from "@/app/components/CompareButton";
import { RatingStars } from "@/app/components/RatingStars";
import { WishlistButton } from "@/app/components/WishlistButton";
import { RequiredFlag } from "./RequiredFlag";
import { MissingOptionLinks } from "./MissingOptionLinks";
import { jumpToOption } from "./focus-intent";

const LIST_VARIANT_ANCHOR = "list-variant";
import { AddToCartButton } from "./AddToCartButton";
import { StickyBuyRow } from "./StickyBuyRow";
import { AddToCartFeedback } from "./AddToCartFeedback";
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
import {
  ListVariantPicker,
  matchVariant,
  VariantPicker,
  type Selection,
} from "./VariantPicker";

// Build the attribute-mode selection corresponding to a specific child: for each
// configurable option, the value that child carries
function seedSelectionFromVariant(
  product: ProductDetail,
  variantId: string | undefined,
): Selection {
  if (!variantId) return {};
  const child = product.children.find((c) => c.id === variantId);
  if (!child) return {};
  const sel: Selection = {};
  for (const opt of product.configurableOptions) {
    const av = child.attributeValues.find((x) => x.attribute.id === opt.id);
    const valueId = av?.selectedValues[0]?.id;
    if (valueId) sel[opt.id] = valueId;
  }
  return sel;
}

export function ConfigurablePDP({
  product,
  initialVariantId,
  variantOwnUrl = false,
}: {
  product: ProductDetail;
  initialVariantId?: string;
  variantOwnUrl?: boolean;
}) {
  const [addState, formAction] = useActionState(addSimpleToCart, null);
  const [cfgState, cfgFormAction] = useActionState(addConfiguredToCart, null);
  const isList = product.variantMode === "LIST";

  const [selection, setSelection] = useState<Selection>(() =>
    isList ? {} : seedSelectionFromVariant(product, initialVariantId),
  );
  const [selectedChildId, setSelectedChildId] = useState<string | null>(() =>
    isList && initialVariantId ? initialVariantId : null,
  );

  // A plain setState commits synchronously, and React only runs <ViewTransition> for ASYNC updates -
  // so without this wrapper the price would just pop to the new variant's. Marking the pick as a
  // transition is what lets the swap animate; it changes nothing else about the update
  //
  // The browser's own root cross-fade rides along, which is the default for any view transition and
  // what the Next guide's same-route example leaves in place - the page barely changes, so it reads
  // as nothing. Scoping it away needs :active-view-transition-type(), and Lightning CSS silently
  // DROPS that selector, so the rule never reaches the stylesheet
  const pickVariant = (next: Selection) => startTransition(() => setSelection(next));
  const pickListVariant = (id: string | null) =>
    startTransition(() => setSelectedChildId(id));

  const matched = useMemo(() => {
    if (isList) {
      return product.children.find((c) => c.id === selectedChildId) ?? null;
    }
    return matchVariant(product.configurableOptions, product.children, selection);
  }, [
    isList,
    product.children,
    product.configurableOptions,
    selection,
    selectedChildId,
  ]);

  useEffect(() => {
    if (!matched) return;
    const title =
      matched.metaTitle ??
      matched.name ??
      product.metaTitle ??
      product.name ??
      product.sku;
    document.title = title;
  }, [matched, product]);

// Mirror the picked variant in the URL via History API only - router.replace would re-fetch when we just want the address bar to track.
// replaceState so option clicks don't flood Back. No match reverts to the parent URL. Operator-gated
  useEffect(() => {
    if (!variantOwnUrl) return;
    const target = matched ? productHref(matched) : productHref(product);
    if (window.location.pathname !== target) {
      window.history.replaceState(window.history.state, "", target);
    }
  }, [matched, variantOwnUrl, product]);

  const heading = displayName(matched?.name ?? product.name, product.sku);
  const displaySku = matched?.sku ?? product.sku;
  // Channel-effective price first - defaultPrice is the base and misses listing
  // overrides and specials, which the cart WILL charge
  const displayPrice = matched
    ? (matched.price ?? matched.defaultPrice)
    : (product.price ?? product.defaultPrice);
  const displayCurrency =
    matched?.defaultCurrencyCode ?? product.defaultCurrencyCode;
  const displayShortDesc =
    matched?.shortDescription ?? product.shortDescription;
  const displayDesc = matched?.description ?? product.description;
  const displayInStock = matched?.inStock ?? product.inStock;
  const displaySalable = matched ? (matched.salableQuantity ?? null) : null;
  const qtyBounds = deriveQtyBounds({
    salableQuantity: matched?.salableQuantity ?? product.salableQuantity,
    maxOrderQty: product.maxOrderQty,
    minOrderQty: product.minOrderQty,
    quantityStep: product.quantityStep,
    quantityDecimals: product.quantityDecimals,
  });
  const buyDisabled = !matched || !matched.inStock;
  const outOfStock = !!matched && !matched.inStock;
  const [attemptSeq, setAttemptSeq] = useState(0);
  const attempted = attemptSeq > 0;
  // Flag the gaps AND go to the first one - the picker is often above the fold on desktop but
  // well out of sight on a phone, where a button that only changes its own label reads as broken
  const onBlockedAdd = () => {
    setAttemptSeq((n) => n + 1);
    const first = !matched
      ? isList
        ? LIST_VARIANT_ANCHOR
        : unansweredVariantOptions[0]?.id
      : opts.missingRequiredItems[0]?.id;
    if (first) jumpToOption(first);
  };
  const optBasePrice = matched
    ? (matched.price ?? matched.defaultPrice)
    : (product.price ?? product.defaultPrice);
  // Variant options from the API already include the parent's (merged, sorted) - dedupe
  // by id so a matched variant doesn't render the parent's options twice
  const combinedOptions = useMemo(() => {
    const seen = new Set<string>();
    return [...product.options, ...(matched?.options ?? [])].filter((o) => {
      if (seen.has(o.id)) return false;
      seen.add(o.id);
      return true;
    });
  }, [product.options, matched]);
  const opts = useConfiguredOptions(combinedOptions, {
    // Percent deltas are computed off defaultPrice server-side - never the
    // channel/special price - so the preview must match
    defaultPrice: matched?.defaultPrice ?? product.defaultPrice,
    effectivePrice: optBasePrice,
    productSku: matched?.sku ?? "",
  });
  const unansweredVariantOptions = product.configurableOptions
    .filter((o) => !selection[o.id])
    .map((o) => ({ id: o.id, label: o.name }));

  const hasOptions = combinedOptions.length > 0;
  // Configured add needs a matched, in-stock variant AND every required option
  // satisfied, with no upload still in flight
  const configuredDisabled =
    buyDisabled || opts.missingRequired.length > 0 || opts.uploading;
  // For attributes, fall back to parent's whole list when the variant didn't expose any
  // - flipping field-by-field would interleave empty rows
  const displayAttrs =
    matched && matched.attributeValues.length > 0
      ? matched.attributeValues
      : product.attributeValues;
  // Same shape for media: a variant either has its own gallery or it doesn't;
  // mix-and-match would shuffle images on every pick
  const displayMedia =
    matched && matched.media.length > 0 ? matched.media : product.media;
  const displayHero =
    matched?.mainImage ?? product.mainImage ?? displayMedia[0] ?? null;

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
          media={displayMedia}
          fallback={displayHero}
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
          <p className="text-sm text-muted-foreground">SKU {displaySku}</p>
          <WishlistButton productId={product.id} size="sm" />
          <CompareButton productId={product.id} />
        </div>

        <div className="glass mt-5 rounded-xl p-6">
        {/* Price and availability are what a shopper is checking when they switch variants, and they
            used to change with no signal at all - a size going from $49 to $59 just silently redrew.
            Keying on the matched child makes the two states an exit/enter pair, so the numbers
            crossfade and the change is visible without shouting
            The gallery needs no wrapper of its own: its first slide already carries a stable
            view-transition name for the grid morph, so when a variant brings its OWN photos the
            remount pairs on that same name and crossfades too */}
        <ViewTransition
          key={matched?.id ?? "base"}
          name="variant-price"
          share="auto"
          enter="auto"
          default="none"
        >
          <div>
            <p className="text-2xl font-bold tracking-tight">
              {formatPrice(displayPrice, displayCurrency)}
            </p>

            <StockBadge
              inStock={displayInStock}
              salableQuantity={displaySalable}
            />
          </div>
        </ViewTransition>

        <section className="mt-6">
          {/* Highlighted when the shopper tries to add without a variant. */}
          <RequiredFlag
            active={attempted && !matched}
            attemptSeq={attemptSeq}
            anchorId={isList ? optionAnchorId(LIST_VARIANT_ANCHOR) : undefined}
          >
            {(invalid) =>
              isList ? (
                <ListVariantPicker
                  variants={product.children}
                  selectedId={selectedChildId}
                  onChange={pickListVariant}
                  invalid={invalid}
                />
              ) : (
                <VariantPicker
                  options={product.configurableOptions}
                  variants={product.children}
                  selection={selection}
                  onChange={pickVariant}
                  invalid={invalid}
                />
              )
            }
          </RequiredFlag>
          {!matched && (
            <p className="mt-3 text-xs text-muted-foreground">
              {isList
                ? "Choose a variant to add to cart."
                : product.configurableOptions.some((o) => !selection[o.id])
                  ? `Pick ${product.configurableOptions
                      .filter((o) => !selection[o.id])
                      .map((o) => o.name)
                      .join(" + ")} to see the matching variant.`
                  : "No variant matches this combination."}
            </p>
          )}

          {/* Parent's product-wide options first, then the matched variant's own */}
          {hasOptions && (
            <div className="mt-6 space-y-6">
              {opts.visibleOptions.map((opt) => (
                <RequiredFlag
                  key={opt.id}
                  active={
                    attempted && !!matched && opts.missingRequiredIds.includes(opt.id)
                  }
                  attemptSeq={attemptSeq}
                  anchorId={optionAnchorId(opt.id)}
                >
                  {(invalid) => (
                  <OptionControl
                    option={opt}
                    currency={displayCurrency}
                    picked={opts.sel[opt.id] ?? new Set()}
                    textValue={opts.text[opt.id] ?? ""}
                    file={opts.files[opt.id]}
                    onToggle={(vid) => opts.toggleValue(opt, vid)}
                    onText={(v) => opts.setOptionText(opt.id, v)}
                    onFile={(e) => opts.onFileChange(opt, e)}
                    invalid={invalid}
                    fileDisabled={!matched}
                    fileDisabledHint="Pick your options first"
                  />
                  )}
                </RequiredFlag>
              ))}
              {opts.forcedQtyConflict && (
                <p role="alert" className="text-xs font-medium text-red-600 dark:text-red-400">
                  These choices set different fixed quantities - keep only one of them.
                </p>
              )}
              {opts.optionsDelta !== 0 && (
                <p className="text-xs text-muted-foreground">
                  {formatPrice(optBasePrice, displayCurrency)} base{" + "}
                  {formatPrice(opts.optionsDelta, displayCurrency)} options{" = "}
                  <span className="font-medium text-foreground">
                    {formatPrice(opts.configuredTotal, displayCurrency)}
                  </span>
                </p>
              )}
            </div>
          )}

          {/* Variants are SIMPLE in the cart - with options we submit the configured line
              (variant SKU + parent and variant picks) so surcharges and required options hold */}
          {hasOptions ? (
            <StickyBuyRow className="mt-6">
            <form action={cfgFormAction} className="flex items-center gap-3">
              <input type="hidden" name="productSku" value={matched?.sku ?? ""} />
              <input
                type="hidden"
                name="productName"
                value={matched?.name ?? matched?.sku ?? ""}
              />
              <input type="hidden" name="options" value={JSON.stringify(opts.payload)} />
              {opts.forcedQty != null ? (
                // A picked option forces the line quantity - submit it and hide the
                // stepper (mirrors CustomOptionsPDP)
                <input type="hidden" name="quantity" value={opts.forcedQty} />
              ) : (
                <QtySelector
                  key={matched?.id ?? "none"}
                  name="quantity"
                  defaultValue={qtyBounds.defaultValue}
                  min={qtyBounds.min}
                  max={qtyBounds.max}
                  step={qtyBounds.step}
                  decimals={qtyBounds.decimals}
                  disabled={configuredDisabled}
                />
              )}
              <AddToCartButton
                stickyPrice={formatPrice(displayPrice, displayCurrency)}
                disabled={outOfStock || opts.uploading}
                blocked={
                  !matched ||
                  opts.missingRequired.length > 0 ||
                  opts.forcedQtyConflict
                }
                onBlockedClick={onBlockedAdd}
                className="flex-1"
              >
                {opts.uploading
                  ? "Uploading…"
                  : !matched
                    ? "Select options"
                    : !matched.inStock
                      ? "Out of stock"
                      : opts.missingRequired.length > 0
                        ? "Complete required options"
                        : opts.forcedQtyConflict
                          ? "Resolve quantity conflict"
                          : "Add to cart"}
              </AddToCartButton>
            </form>
            </StickyBuyRow>
          ) : (
            <StickyBuyRow className="mt-6">
            <form action={formAction} className="flex items-center gap-3">
              <input
                type="hidden"
                name="productSku"
                value={matched?.sku ?? ""}
              />
              <input
                type="hidden"
                name="productName"
                value={matched?.name ?? matched?.sku ?? ""}
              />
              <input
                type="hidden"
                name="unitPrice"
                value={matched?.defaultPrice ?? 0}
              />
              {/* key remounts on variant change so the stepper resets and picks up the new stock ceiling */}
              <QtySelector
                key={matched?.id ?? "none"}
                name="quantity"
                defaultValue={qtyBounds.defaultValue}
                min={qtyBounds.min}
                max={qtyBounds.max}
                step={qtyBounds.step}
                decimals={qtyBounds.decimals}
                disabled={buyDisabled}
              />
              <AddToCartButton
                stickyPrice={formatPrice(displayPrice, displayCurrency)}
                disabled={outOfStock}
                blocked={!matched}
                onBlockedClick={onBlockedAdd}
                className="flex-1"
              >
                {!matched
                  ? "Select options"
                  : !matched.inStock
                    ? "Out of stock"
                    : "Add to cart"}
              </AddToCartButton>
            </form>
            </StickyBuyRow>
          )}
          {attempted && !matched ? (
            <SelectionHint key={attemptSeq}>
              {isList ? (
                <>
                  Still to choose:{" "}
                  <MissingOptionLinks
                    items={[{ id: LIST_VARIANT_ANCHOR, label: "a variant" }]}
                  />
                </>
              ) : unansweredVariantOptions.length > 0 ? (
                <>
                  Still to choose:{" "}
                  <MissingOptionLinks items={unansweredVariantOptions} />
                </>
              ) : (
                "That combination isn't available. Try a different one."
              )}
            </SelectionHint>
          ) : attempted &&
            hasOptions &&
            matched?.inStock &&
            opts.missingRequired.length > 0 ? (
            <SelectionHint key={attemptSeq}>
              Required option{opts.missingRequired.length > 1 ? "s" : ""}:{" "}
              <MissingOptionLinks items={opts.missingRequiredItems} />
            </SelectionHint>
          ) : null}
          <AddToCartFeedback state={hasOptions ? cfgState : addState} />
        </section>

        <ProductReassurance className="mt-5 border-t border-border pt-4" />
        </div>

        <Description
          label="Summary"
          value={displayShortDesc}
          className="mt-8"
        />
        <Description
          label="Description"
          value={displayDesc}
          className="mt-6"
        />

        <AttributeList values={displayAttrs} className="mt-8" />

        {(product.categories.length > 0 || product.collections.length > 0) && (
          <section className="mt-8 text-sm">
            {product.primaryCategory && (
              <p className="text-muted-foreground">
                Primary category:{" "}
                <span className="text-foreground">
                  {displayName(
                    product.primaryCategory.name,
                    product.primaryCategory.slug,
                  )}
                </span>
              </p>
            )}
            {product.collections.length > 0 && (
              <p className="mt-1 text-muted-foreground">
                Collections:{" "}
                <span className="text-foreground">
                  {product.collections
                    .map((c) => displayName(c.name, c.slug))
                    .join(", ")}
                </span>
              </p>
            )}
          </section>
        )}
      </div>
    </div>
  );
}
