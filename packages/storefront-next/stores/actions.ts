"use server";
import "../setup";

import * as core from "@skelet/storefront-core/stores/actions";

export type * from "@skelet/storefront-core/stores/actions";

export async function findStoresAction(...args: Parameters<typeof core.findStoresAction>) {
  return core.findStoresAction(...args);
}

export async function searchStoreProductsAction(...args: Parameters<typeof core.searchStoreProductsAction>) {
  return core.searchStoreProductsAction(...args);
}

export async function pickStoreProductAction(...args: Parameters<typeof core.pickStoreProductAction>) {
  return core.pickStoreProductAction(...args);
}
