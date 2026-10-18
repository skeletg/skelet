import type { AppliedPromotionCode, PromotionFreeGift } from "@/app/lib/cart/types";

// What an applied promotion actually DOES for this cart. A code's benefit isn't
// always a subtotal discount - free shipping and free gifts both leave
// subtotalDiscount at 0 - so anything rendering a money amount has to ask first.
// The checkout summary printed subtotalDiscount unconditionally and showed
// "-$0.00" for those, which reads as a broken discount.
export type AppliedCodeBenefit = {
  // Code for a typed coupon, else the rule's public label (auto-apply rules have no code)
  label: string;
  // Secondary line, empty when it would just repeat the label
  description: string;
  // Non-empty when the code was valid at apply time but isn't now
  invalidatedReason: string;
  // 0 means "don't render a money line" - not "the discount is zero"
  discountCents: number;
  freeShipping: boolean;
  // Gifts actually in the cart, not the ones still on offer
  giftsInCart: PromotionFreeGift[];
  // False when the code currently does nothing visible - an invalidated code, or
  // one whose only offer is a gift the shopper hasn't added yet
  hasVisibleBenefit: boolean;
};

export function describeAppliedCode(c: AppliedPromotionCode): AppliedCodeBenefit {
  const invalidatedReason = c.invalidatedReason ?? "";
  const invalidated = invalidatedReason !== "";
  const giftsInCart = invalidated ? [] : c.freeGifts.filter((g) => g.inCart);
  const discountCents = invalidated ? 0 : c.subtotalDiscount;
  const freeShipping = !invalidated && c.freeShipping;
  const fallback = c.publicLabel || c.ruleName;
  return {
    label: c.code || fallback,
    description: c.code ? fallback : "",
    invalidatedReason,
    discountCents,
    freeShipping,
    giftsInCart,
    hasVisibleBenefit:
      discountCents > 0 || freeShipping || giftsInCart.length > 0,
  };
}

// "2× TOTE-BAG, MUG" - the gifts a code actually put in the cart
export function giftSummary(gifts: PromotionFreeGift[]): string {
  return gifts
    .map((g) => `${g.quantity > 1 ? `${g.quantity}× ` : ""}${g.sku}`)
    .join(", ");
}
