"use client";

import { StoreAvailability } from "@/app/components/stores/StoreAvailability";

import { useActionState, useState } from "react";
import { useTranslations } from "next-intl";

import { addConfiguredToCart } from "@/app/lib/cart/actions";
import { displayName } from "@/app/lib/display";
import { formatPrice } from "@/app/lib/format";
import type { ProductDetail } from "@/app/lib/catalog/read";
import { ApproxPrice } from "@/app/components/ApproxPrice";
import { deriveQtyBounds } from "@/app/components/qty";
import { RequiredFlag } from "./RequiredFlag";
import { MissingOptionLinks } from "./MissingOptionLinks";
import { jumpToOption } from "./focus-intent";
import { AddToCartButton } from "@/app/components/product/AddToCartButton";
import { StickyBuyRow } from "@/app/components/product/StickyBuyRow";
import { AddToCartFeedback } from "@/app/components/product/AddToCartFeedback";
import { OptionControl } from "./ProductOptions";
import { useConfiguredOptions } from "./useConfiguredOptions";
import {
  AttributeList,
  Description,
  optionAnchorId,
  SelectionHint,
  StockBadge,
} from "./parts";
import {
  OptionsAddLabel,
  PdpBuyBox,
  PdpMetaRow,
  PdpQty,
  PdpShell,
} from "./pdp-shell";

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
    <PdpShell product={product} heading={heading}>
      <PdpMetaRow product={product} />

      <PdpBuyBox>
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
          <PdpQty
            bounds={qtyBounds}
            forcedQty={opts.forcedQty}
            disabled={!product.inStock}
          />
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
            <OptionsAddLabel
              uploading={opts.uploading}
              missingRequired={opts.missingRequired.length > 0}
              forcedQtyConflict={opts.forcedQtyConflict}
            />
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

        <StoreAvailability productId={product.id} productName={heading} />
      </PdpBuyBox>

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
    </PdpShell>
  );
}
