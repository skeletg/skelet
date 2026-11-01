"use server";
import "../setup";

import * as core from "@skelet/storefront-core/wishlist/actions";

export async function toggleWishlistAction(...args: Parameters<typeof core.toggleWishlistAction>) {
  return core.toggleWishlistAction(...args);
}

export async function wishlistIdsAction(...args: Parameters<typeof core.wishlistIdsAction>) {
  return core.wishlistIdsAction(...args);
}

export async function moveToCartAction(...args: Parameters<typeof core.moveToCartAction>) {
  return core.moveToCartAction(...args);
}
