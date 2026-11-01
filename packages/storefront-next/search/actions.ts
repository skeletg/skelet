"use server";
import "../setup";

import * as core from "@skelet/storefront-core/search/actions";

export async function searchProductsAction(...args: Parameters<typeof core.searchProductsAction>) {
  return core.searchProductsAction(...args);
}
