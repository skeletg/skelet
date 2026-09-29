"use server";

import { revalidatePath } from "next/cache";

import {
  AddToWishlistDocument,
  MoveToCartDocument,
  RemoveFromWishlistDocument,
  WishlistProductIdsDocument,
} from "@/app/lib/wishlist/operations.gql";
import { getCart } from "@/app/lib/cart/read";
import type { Cart } from "@/app/lib/cart/types";
import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";

// The wishlist page, in whichever channel and locale it renders - only it lists the saved items
const WISHLIST_ROUTE = "/[channel]/[locale]/account/wishlist";

export async function toggleWishlistAction(
  productId: string,
  add: boolean,
): Promise<{ authed: boolean }> {
  try {
    if (add) {
      await gqlFetch(AddToWishlistDocument, { productId });
    } else {
      await gqlFetch(RemoveFromWishlistDocument, { productId });
    }
    revalidatePath(WISHLIST_ROUTE, "page");
    return { authed: true };
  } catch {
    // UNAUTHENTICATED (guest) or transient failure - client keeps local
    return { authed: false };
  }
}

export async function wishlistIdsAction(): Promise<string[]> {
  try {
    const res = await gqlFetch(WishlistProductIdsDocument, {});
    return res.data.wishlistProductIds;
  } catch {
    return [];
  }
}

// moveToCartAction moves a wishlist item into the cart (add + remove from wishlist). The header's cart badge reads the
// client cart store, not the layout, so the updated cart goes back to the caller instead of revalidating every page
export async function moveToCartAction(
  productId: string,
): Promise<{ ok: boolean; error?: string; cart?: Cart | null }> {
  try {
    await gqlFetch(MoveToCartDocument, { productId });
    revalidatePath(WISHLIST_ROUTE, "page");
    return { ok: true, cart: await getCart() };
  } catch (err) {
    let msg = "moveFailed";
    if (err instanceof GraphQLRequestError) {
      const code = err.errors[0]?.extensions?.code;
      msg =
        code === "PRODUCT_OPTIONS_REQUIRED"
          ? "needsOptions"
          : (err.errors[0]?.message ?? msg).replace(/^add to cart(?: rejected)?:\s*/i, "");
    }
    return { ok: false, error: msg };
  }
}
