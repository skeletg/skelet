import type { ProductSummary } from "@skelet/storefront-next/catalog/read";

export type PriceView = { pricedBySelection: boolean; sellPrice: number; compareAt: number | null; savePct: number };

// How a product's price reads on a card or panel. A dynamically priced bundle carries no price of its own - the cart and
// the PDP total its picked selections - so with no selections to total it quotes no price rather than "$0.00". The
// savings badge starts at 5% so rounding noise doesn't make a "Save 1%" sticker
export function priceView(product: Pick<ProductSummary, "price" | "defaultPrice" | "compareAtPrice" | "kind">): PriceView {
  const sellPrice = product.price ?? product.defaultPrice;
  const pricedBySelection = sellPrice <= 0 && product.kind !== "SIMPLE";
  const compareAt = !pricedBySelection && product.compareAtPrice && product.compareAtPrice > sellPrice ? product.compareAtPrice : null;
  return { pricedBySelection, sellPrice, compareAt, savePct: compareAt ? Math.round((1 - sellPrice / compareAt) * 100) : 0 };
}
