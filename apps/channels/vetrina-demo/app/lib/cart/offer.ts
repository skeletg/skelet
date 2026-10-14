// Offer cart reads - an offer is a cart the store built for one shopper and sent as a link. It is never the
// shopper's own cart: it lives beside it, and checkout works on it only while OFFER_CHECKOUT_COOKIE says so

import { cache } from "react";
import { cookies } from "next/headers";

import { CartByIdDocument } from "@/app/lib/cart/queries.gql";
import { decodeOfferSession, OFFER_CHECKOUT_COOKIE, OFFER_COOKIE, type OfferSession } from "@/app/lib/cart/offer-cookie";
import { getCart } from "@/app/lib/cart/read";
import type { Cart } from "@/app/lib/cart/types";
import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";

export async function getOfferSession(): Promise<OfferSession | null> {
  const store = await cookies();
  return decodeOfferSession(store.get(OFFER_COOKIE)?.value);
}

export type OfferRead = { cart: Cart; state: null } | { cart: null; state: "invalid" | "expired" | "revoked" };

// The offer this browser holds - or why it can't be opened: none held, a replaced link or a finished offer read as
// invalid, and an expired or withdrawn one says so
export const readOffer = cache(async (): Promise<OfferRead> => {
  const session = await getOfferSession();
  if (!session) return { cart: null, state: "invalid" };
  try {
    const res = await gqlFetch(CartByIdDocument, { id: session.cartId });
    return res.data.cart?.offer ? { cart: res.data.cart, state: null } : { cart: null, state: "invalid" };
  } catch (err) {
    const code = err instanceof GraphQLRequestError ? err.errors[0]?.extensions?.code : undefined;
    if (code === "CART_OFFER_EXPIRED") return { cart: null, state: "expired" };
    if (code === "CART_OFFER_REVOKED") return { cart: null, state: "revoked" };
    return { cart: null, state: "invalid" };
  }
});

// The offer this browser holds, or null when there is none or it can no longer be opened
export async function getOfferCart(): Promise<Cart | null> {
  return (await readOffer()).cart;
}

export async function isOfferCheckout(): Promise<boolean> {
  const store = await cookies();
  return store.get(OFFER_CHECKOUT_COOKIE)?.value === "1" && !!(await getOfferSession());
}

// The cart checkout works on - the offer while the shopper is completing one, else their own
export const getCheckoutCart = cache(async (): Promise<Cart | null> => {
  if (await isOfferCheckout()) return getOfferCart();
  return getCart();
});
