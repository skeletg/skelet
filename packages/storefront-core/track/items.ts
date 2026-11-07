import type { CartLine } from "../cart/types";
import { displayName } from "../display";
import { toMajor, type CurrencyInfo } from "../format";

// A product as a GA4 item - the SKU as its id, as the server's cart and purchase events have it, at the price shown
export type ItemSource = {
  sku: string;
  name: string | null;
  price?: number | null;
  defaultPrice: number;
  defaultCurrencyCode: string;
};

export type ItemList = { id: string; name: string };

export function ga4Item(p: ItemSource, index?: number, list?: ItemList) {
  return {
    item_id: p.sku,
    item_name: p.name ?? undefined,
    price: toMajor(p.price ?? p.defaultPrice, p.defaultCurrencyCode),
    index,
    item_list_id: list?.id,
    item_list_name: list?.name,
  };
}

// A cart's lines as GA4's ecommerce block - view_cart and begin_checkout, valued at what the page shows
export function ga4CartEcommerce(lines: CartLine[], valueMinor: number, currency: string | CurrencyInfo) {
  return {
    currency: typeof currency === "string" ? currency : currency.code,
    value: toMajor(valueMinor, currency),
    items: lines.map((line) => ({
      item_id: line.sku,
      item_name: displayName(line.name, line.sku),
      quantity: line.quantity,
      price: toMajor(line.unitPrice, currency),
    })),
  };
}
