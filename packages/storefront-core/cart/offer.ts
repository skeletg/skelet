// Offer cart reads - an offer is a cart the store built for one shopper and sent as a link. It is never the
// shopper's own cart: it lives beside it, and checkout works on it only when its id is explicitly selected


import { perRequest, request } from "../request";
import { CartByIdDocument } from "./queries.gql";
import { decodeOfferSession, OFFER_COOKIE, type OfferSession } from "./offer-cookie";
import { getCart } from "./read";
import type { Cart } from "./types";
import { gqlFetch, GraphQLRequestError } from "../graphql";

export async function getOfferSession(): Promise<OfferSession | null> {
  const store = await request().cookies();
  return decodeOfferSession(store.get(OFFER_COOKIE)?.value);
}

export type OfferRead = { cart: Cart; state: null } | { cart: null; state: "invalid" | "expired" | "revoked" };

// The offer this browser holds - or why it can't be opened: none held, a replaced link or a finished offer read as
// invalid, and an expired or withdrawn one says so
export const readOffer = perRequest(async (cartId?: string): Promise<OfferRead> => {
  const session = await getOfferSession();
  if (!session && !cartId) return { cart: null, state: "invalid" };
  try {
    const res = await gqlFetch(CartByIdDocument, { id: cartId ?? session!.cartId });
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

// No explicit id means the shopper's own cart, regardless of offer cookies in another tab.
// An explicit id is reauthorized by the backend with the current session/link credentials.
export const getCheckoutCart = perRequest(async (cartId?: string): Promise<Cart | null> => {
  if (cartId === undefined) return getCart();
  if (!cartId) return null;
  try {
    return (await gqlFetch(CartByIdDocument, { id: cartId })).data.cart;
  } catch {
    return null;
  }
});
