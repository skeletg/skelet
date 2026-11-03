// Cart read helpers

import { perRequest, request, rethrowControlFlow } from "../request";
import {
  CartByIdDocument,
  FreeShippingThresholdDocument,
  MyCartDocument,
} from "./queries.gql";
import { getAccessToken } from "../auth/session";
import type { Cart } from "./types";
import { errorClass, gqlFetch } from "../graphql";
import { surfaceCacheOpts, NAV_SURFACE, NAV_TAG } from "../config/read";

export { cartLineCount } from "./types";
export type {
  Cart,
  CartLine,
  CartLineComponent,
  AppliedPromotionCode,
  PromotionFreeGift,
} from "./types";

export const CART_COOKIE = "storefront_cart";
export { PROMO_PENDING_COOKIE } from "../promo";

// The shopper's own cart id: the session's when signed in (the cart cookie is ignored), else the guest cookie's, else
// null. Never an offer, and a failed backend read throws rather than handing back some other cart
export async function getOwnCartId(): Promise<string | null> {
  if (await getAccessToken()) return (await gqlFetch(MyCartDocument)).data.myCart.id;
  return (await request().cookies()).get(CART_COOKIE)?.value ?? null;
}

// unavailable says the read failed - only a null cart or no cookie means a guest's cart is gone
export type CartRead = { cart: Cart | null; unavailable: boolean };

export const readCart = perRequest(async (): Promise<CartRead> => {
  // Signed in: the cart hangs off the session, not the cookie, so it's the same cart on
  // every device the customer opens. A failed read is no cart, never the guest cookie's
  if (await getAccessToken()) {
    try {
      return { cart: (await gqlFetch(MyCartDocument)).data.myCart, unavailable: false };
    } catch (err) {
      return failedRead(err);
    }
  }
  const id = (await request().cookies()).get(CART_COOKIE)?.value;
  if (!id) return { cart: null, unavailable: false };
  try {
    return { cart: (await gqlFetch(CartByIdDocument, { id })).data.cart, unavailable: false };
  } catch (err) {
    return failedRead(err);
  }
});

function failedRead(err: unknown): CartRead {
  rethrowControlFlow(err);
  return { cart: null, unavailable: errorClass(err) !== "NOT_FOUND" };
}

export const getCart = perRequest(async (): Promise<Cart | null> => (await readCart()).cart);

export { formatPrice } from "../format";

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
