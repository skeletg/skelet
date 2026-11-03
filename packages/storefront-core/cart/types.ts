// Pure cart types + value helpers

export type CartLineComponent = {
  id: string;
  optionId: string;
  selectionId: string;
  sku: string;
  name: string;
  unitPrice: number;
  quantity: number;
  sortOrder: number;
};

export type CartLineOption = {
  id: string;
  productOptionId: string;
  optionValueId?: string | null;
  inputType: string;
  optionTitle: string;
  valueLabel: string;
  textValue: string;
  priceDelta: number;
  quantity: number;
  // Public URL of an uploaded file for FILE-type options (empty otherwise); textValue
  // carries the original filename
  fileUrl: string;
};

export type CartLine = {
  id: string;
  name: string;
  sku: string;
  imageUrl?: string | null;
  unitPrice: number;
  quantity: number;
  totalPrice: number;
  availableQuantity?: number | null;
  currentUnitPrice?: number | null;
  components: CartLineComponent[];
  options: CartLineOption[];
  // A charge the store added to an offer by hand - no product page, nothing to deliver
  isCustom?: boolean;
};

export type AppliedPromotionCode = {
  code: string;
  ruleName: string;
  publicLabel: string;
  subtotalDiscount: number;
  freeShipping: boolean;
  // Populated when a code was applied earlier but is no longer valid (max_uses hit,
  // expired window, condition no longer matches)
  invalidatedReason: string;
  requiresCode: boolean;
  freeGifts: PromotionFreeGift[];
};

export type PromotionFreeGift = {
  // Null when the SKU doesn't resolve to a product on the current store
  productId: string | null;
  sku: string;
  quantity: number;
  inCart: boolean;
  href: string | null;
};

export type SavedItem = {
  id: string;
  sku: string;
  name: string;
  imageUrl?: string | null;
  quantity: number;
};

export type Cart = {
  id: string;
  note: string;
  subtotal: number;
  totalDiscount: number;
  appliedCodes: AppliedPromotionCode[];
  lines: CartLine[];
  savedItems: SavedItem[];
  // Set on an offer the store built and sent - null on the shopper's own cart
  offer?: CartOffer | null;
};

export type CartOfferLock = "LINES" | "BILLING" | "SHIPPING" | "DELIVERY" | "COUPONS";
export type CartOfferState = "DRAFT" | "SENT" | "OPENED" | "COMPLETED" | "EXPIRED" | "REVOKED";

export type CartOffer = {
  expiresAt: string;
  message: string;
  state: CartOfferState;
  locks: CartOfferLock[];
  paymentMethods: string[];
  isGuest: boolean;
};

export function offerLocks(cart: Pick<Cart, "offer"> | null | undefined, lock: CartOfferLock): boolean {
  return !!cart?.offer?.locks.includes(lock);
}

export function cartLineCount(cart: Cart | null): number {
  if (!cart) return 0;
  return cart.lines.reduce((n, l) => n + l.quantity, 0);
}
