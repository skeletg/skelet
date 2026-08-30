// Pure cart types + value helpers

export type CartLineComponent = {
  id: string;
  optionId: string;
  selectionId: string;
  productSku: string;
  productName: string;
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
  productName: string;
  productSku: string;
  imageUrl?: string | null;
  unitPrice: number;
  quantity: number;
  totalPrice: number;
  availableQuantity?: number | null;
  currentUnitPrice?: number | null;
  components: CartLineComponent[];
  options: CartLineOption[];
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
  productSku: string;
  quantity: number;
  inCart: boolean;
  href: string | null;
};

export type SavedItem = {
  id: string;
  productSku: string;
  productName: string;
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
};

export function cartLineCount(cart: Cart | null): number {
  if (!cart) return 0;
  return cart.lines.reduce((n, l) => n + l.quantity, 0);
}
