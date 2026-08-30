"use server";

import { revalidatePath } from "next/cache";

import {
  AddToWishlistDocument,
  MoveToCartDocument,
  RemoveFromWishlistDocument,
  WishlistProductIdsDocument,
} from "@/app/lib/wishlist/operations.gql";
import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";

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
    revalidatePath("/account/wishlist");
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

// moveToCartAction moves a wishlist item into the cart (add + remove from wishlist),
// revalidating the wishlist page + header cart
export async function moveToCartAction(
  productId: string,
): Promise<{ ok: boolean; error?: string }> {
  try {
    await gqlFetch(MoveToCartDocument, { productId });
    revalidatePath("/account/wishlist");
    revalidatePath("/", "layout");
    return { ok: true };
  } catch (err) {
    let msg = "Could not move to cart.";
    if (err instanceof GraphQLRequestError) {
      const code = err.errors[0]?.extensions?.code;
      msg =
        code === "PRODUCT_OPTIONS_REQUIRED"
          ? "This product needs options - open its page to choose them."
          : (err.errors[0]?.message ?? msg).replace(/^add to cart(?: rejected)?:\s*/i, "");
    }
    return { ok: false, error: msg };
  }
}
