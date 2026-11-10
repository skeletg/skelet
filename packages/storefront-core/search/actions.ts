// Typed RPC entry so the client search modal can reach searchProducts - gqlFetch is
// server-only, it reads cookies and sets X-Channel-Id from request context

import { productCards, type ProductSummary } from "../catalog/read";
import { searchProducts, type ProductSearchHit } from "./read";

// An overview names a handful of products, so anything longer isn't one
const MAX_OVERVIEW_CARDS = 12;

export async function searchProductsAction(query: string): Promise<{
  items: ProductSearchHit[];
  totalCount: number;
}> {
  return searchProducts(query, { limit: 8 });
}

// The cards an overview's chat opens with, fetched only when the shopper carries it into the chat
export async function overviewCardsAction(ids: unknown): Promise<ProductSummary[]> {
  if (!Array.isArray(ids) || ids.length > MAX_OVERVIEW_CARDS || !ids.every((id) => typeof id === "string" && id !== "")) return [];
  return productCards(ids);
}
