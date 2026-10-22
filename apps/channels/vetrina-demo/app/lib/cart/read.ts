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
// A code that arrived before there was a cart to apply it to - the next addition applies it
export const PROMO_PENDING_COOKIE = "storefront_pending_promo";

// The shopper's own cart id: the session's when signed in (the cart cookie is ignored), else the guest cookie's, else
// null. Never an offer, and a failed backend read throws rather than handing back some other cart
export async function getOwnCartId(): Promise<string | null> {
  if (await getAccessToken()) return (await gqlFetch(MyCartDocument)).data.myCart.id;
  return (await cookies()).get(CART_COOKIE)?.value ?? null;
}

// Returns the shopper's own cart, or null when a guest has none yet or the backend can't read it (cleared, expired,
// never existed under that ID)
export const getCart = cache(async (): Promise<Cart | null> => {
  // Signed in: the cart hangs off the session, not the cookie, so it's the same cart on
  // every device the customer opens. A failed read is no cart, never the guest cookie's
  if (await getAccessToken()) {
    try {
      const res = await gqlFetch(MyCartDocument);
      return res.data.myCart;
    } catch {
      return null;
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
