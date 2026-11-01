// The offer a browser holds - an offer cart is opened through its emailed link, and the link's token rides every
// call for it (X-Cart-Offer). Nothing imported but the cookie policy, so gqlFetch and route handlers can share it without
// a cycle

import { SECURE_COOKIES } from "../cookie-policy";

// httpOnly - the token is what opens the offer, so client JS never reads it
export const OFFER_COOKIE = "storefront_offer";
// Legacy checkout selector, cleared on navigation. Checkout now selects offers by URL
// and pins every action to the rendered cart id, so this cookie no longer selects a cart.
export const OFFER_CHECKOUT_COOKIE = "storefront_checkout_offer";
export const OFFER_HEADER = "X-Cart-Offer";

export type OfferSession = {
  cartId: string;
  // Empty for an offer opened from the account, which its owner may open without the link
  token: string;
};

export function encodeOfferSession(s: OfferSession): string {
  return JSON.stringify(s);
}

export function decodeOfferSession(raw: string | undefined): OfferSession | null {
  if (!raw) return null;
  try {
    const v = JSON.parse(raw) as Partial<OfferSession>;
    if (typeof v.cartId !== "string" || !v.cartId) return null;
    return { cartId: v.cartId, token: typeof v.token === "string" ? v.token : "" };
  } catch {
    return null;
  }
}

export function offerCookieOptions(maxAgeSeconds: number) {
  return {
    path: "/",
    httpOnly: true,
    sameSite: "lax" as const,
    secure: SECURE_COOKIES,
    maxAge: Math.max(60, Math.floor(maxAgeSeconds)),
  };
}
