// Cart read helpers

import { cache } from "react";
import { cookies } from "next/headers";

import {
  CartByIdDocument,
  FreeShippingThresholdDocument,
  MyCartDocument,
} from "@/app/lib/cart/queries.gql";
import { getAccessToken } from "@/app/lib/auth/session";
import type { Cart } from "@/app/lib/cart/types";
import { gqlFetch } from "@/app/lib/graphql";
import { surfaceCacheOpts, NAV_SURFACE, NAV_TAG } from "@/app/lib/config/read";

export { cartLineCount } from "@/app/lib/cart/types";
export type {
  Cart,
  CartLine,
  CartLineComponent,
  AppliedPromotionCode,
  PromotionFreeGift,
} from "@/app/lib/cart/types";

export const CART_COOKIE = "storefront_cart";

// Returns the guest cart pointed to by the cookie, or null if no cookie is set (no cart created yet) or the backend
// says it's gone (cleared, expired, never existed under that ID)
export const getCart = cache(async (): Promise<Cart | null> => {
  // Signed in: the cart hangs off the session, not the cookie, so it's the same cart on
  // every device the customer opens
  if (await getAccessToken()) {
    try {
      const res = await gqlFetch(MyCartDocument);
      return res.data.myCart;
    } catch {
      // Token expired / store mismatch - fall through to the guest cookie rather than
      // showing an empty cart
    }
  }
  const store = await cookies();
  const id = store.get(CART_COOKIE)?.value;
  if (!id) return null;
  try {
    const res = await gqlFetch(CartByIdDocument, { id });
    return res.data.cart;
  } catch {
    // Treat any read failure (404, network blip) as "no cart yet" - the next
    // add-to-cart will create a fresh one
    return null;
  }
});

export { formatPrice } from "@/app/lib/format";

export async function getFreeShippingThreshold(): Promise<number> {
  try {
    // Channel-wide threshold: same for every visitor, changes rarely → cached
    // under the shared "nav" surface. Dev = always fresh
    const cacheOpts = await surfaceCacheOpts(NAV_SURFACE, { tags: [NAV_TAG] });
    const res = await gqlFetch(FreeShippingThresholdDocument, {}, cacheOpts);
    return res.data.freeShippingThreshold ?? 0;
  } catch {
    return 0;
  }
}
