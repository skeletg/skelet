"use client";

import { Fragment, useActionState } from "react";
import { useTranslations } from "next-intl";
import { Link } from "@/app/components/LocaleLink";

import { addSimpleToCart } from "@skelet/storefront-next/cart/actions";
import { productHref } from "@skelet/storefront-core/entity-href";
import { AddToCartButton } from "@/app/components/product/AddToCartButton";
import { StickyBuyRow } from "@/app/components/product/StickyBuyRow";
import { AddToCartFeedback } from "@/app/components/product/AddToCartFeedback";
import { deriveQtyBounds } from "./qty";
import { QtySelector } from "./QtySelector";

type ProductLike = {
  sku: string;
  url: string;
  name: string | null;
  defaultPrice: number;
  inStock: boolean;
  // Only ProductDetail carries this; ProductSummary doesn't
  salableQuantity?: number | null;
  quantityDecimals?: number | null;
  quantityStep?: number | null;
  // Order quantity bounds; server-enforced, mirrored here for UX
  minOrderQty?: number | null;
  maxOrderQty?: number | null;
  backorderable?: boolean | null;
};

export function AddToCartForm({
  product,
  size = "md",
  hideQty = false,
  mobileSticky = false,
  stickyPrice,
}: {
  product: ProductLike;
  size?: "sm" | "md";
  hideQty?: boolean;
  // Re-anchors the row as a phone bottom bar while the in-page row is out of view
  mobileSticky?: boolean;
  stickyPrice?: string;
}) {
  const backorder = !product.inStock && !!product.backorderable;
  const disabled = !product.inStock && !backorder;
  const { min, max, step, decimals, defaultValue } = deriveQtyBounds(product);
  const [state, formAction] = useActionState(addSimpleToCart, null);
  const t = useTranslations("product");
  const tcard = useTranslations("product.card");
  const tc = useTranslations("common");

  // A SIMPLE with required options can't quick-add - present it like the non-simple
  // kinds do instead of echoing the server rejection
  if (state && !state.ok && state.code === "PRODUCT_OPTIONS_REQUIRED") {
    const linkSize =
      size === "sm" ? "h-10 px-5 text-xs" : "h-12 px-7 text-sm";
    return (
      <Link
        href={productHref(product)}
        className={`inline-flex w-full items-center justify-center gap-1.5 rounded-md bg-primary font-semibold text-primary-foreground transition-colors duration-200 hover:bg-primary/85 ${linkSize}`}
      >
        {tcard("chooseOptions")}
      </Link>
    );
  }

  const Row = mobileSticky ? StickyBuyRow : Fragment;
  return (
    <div className="w-full">
    <Row>
    <form
      action={formAction}
      className="flex w-full items-center gap-2"
    >
      <input type="hidden" name="productSku" value={product.sku} />
      {hideQty ? (
        <input type="hidden" name="quantity" value={defaultValue} />
      ) : (
        <QtySelector
          name="quantity"
          defaultValue={defaultValue}
          min={min}
          max={max}
          step={step}
          decimals={decimals}
          disabled={disabled}
          size={size}
        />
      )}
      <AddToCartButton disabled={disabled} size={size} stickyPrice={stickyPrice} className="flex-1">
        {product.inStock
          ? t("addToCart")
          : backorder
            ? t("addToCartBackorder")
            : tc("outOfStock")}
      </AddToCartButton>
    </form>
    </Row>
    <AddToCartFeedback state={state} />
    </div>
  );
}
