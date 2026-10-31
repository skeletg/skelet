"use server";

// Typed RPC entry so the client search modal can reach searchProducts - gqlFetch is
// server-only, it reads cookies and sets X-Channel-Id from request context

import { searchProducts, type ProductSearchHit } from "./read";

export async function searchProductsAction(query: string): Promise<{
  items: ProductSearchHit[];
  totalCount: number;
}> {
  return searchProducts(query, { limit: 8 });
}
