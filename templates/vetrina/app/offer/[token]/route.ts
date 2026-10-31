import { NextRequest, NextResponse } from "next/server";

import { OpenCartOfferDocument } from "@skelet/storefront-next/cart/offer.gql";
import { encodeOfferSession, OFFER_CHECKOUT_COOKIE, OFFER_COOKIE, offerCookieOptions } from "@skelet/storefront-next/cart/offer-cookie";
import { gqlFetch, GraphQLRequestError } from "@skelet/storefront-next/graphql";

// Where an offer email's link lands. A followed link is kept in an httpOnly cookie and the shopper goes on to the
// offer page - an offer made out to an account asks them to sign in first, then comes back here. The backend says
// what the link opens, so a finished, withdrawn or replaced offer gets its own explanation rather than a cart

// The cookie only holds the link - the backend refuses the offer the moment it expires, whatever the cookie says
const OFFER_COOKIE_DAYS = 60;

export async function GET(req: NextRequest, ctx: { params: Promise<{ token: string }> }) {
  const { token: raw } = await ctx.params;
  const token = decodeURIComponent(raw ?? "").trim();
  // req.url carries the container bind address (0.0.0.0) - the redirect goes back to the host the browser asked, so
  // the cookie set here travels with it (the same reading app/masquerade makes)
  const host = req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? req.nextUrl.host;
  const proto = req.headers.get("x-forwarded-proto") ?? "http";
  const to = (path: string) => NextResponse.redirect(new URL(path, `${proto}://${host}`), 307);
  if (!token) return to("/cart/offer?state=invalid");

  let opening;
  try {
    opening = (await gqlFetch(OpenCartOfferDocument, { token })).data.openCartOffer;
  } catch (err) {
    const code = err instanceof GraphQLRequestError ? err.errors[0]?.extensions?.code : undefined;
    return to(`/cart/offer?state=${code === "CART_OFFER_OTHER_ACCOUNT" ? "other-account" : "invalid"}`);
  }
  if (opening.state === "COMPLETED" || opening.state === "EXPIRED" || opening.state === "REVOKED") {
    return to(`/cart/offer?state=${opening.state.toLowerCase()}`);
  }
  // An offer made out to an account keeps its link while its owner signs in - the link alone still opens nothing
  // without them, and sign-in returns to the offer page, a page the login's redirect can reach (this route isn't one)
  const res = to(opening.requiresLogin ? `/account/login?returnTo=${encodeURIComponent("/cart/offer")}` : "/cart/offer");
  res.cookies.set(OFFER_COOKIE, encodeOfferSession({ cartId: opening.cartId, token }), offerCookieOptions(OFFER_COOKIE_DAYS * 24 * 60 * 60));
  // A fresh link starts on the offer, not halfway through a checkout of something else
  res.cookies.delete(OFFER_CHECKOUT_COOKIE);
  return res;
}
