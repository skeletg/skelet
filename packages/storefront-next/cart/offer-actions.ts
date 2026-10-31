"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { refresh } from "next/cache";

import { RemoveFromCartDocument, UpdateCartLineDocument } from "./mutations.gql";
import { MarkCartOfferOpenedDocument, MyCartOffersDocument } from "./offer.gql";
import { encodeOfferSession, OFFER_CHECKOUT_COOKIE, OFFER_COOKIE, offerCookieOptions } from "./offer-cookie";
import { checkoutHref } from "../checkout/path";
import { getCheckoutCart, getOfferSession } from "./offer";
import { gqlFetch } from "../graphql";
import { withLocalePrefix } from "../locale/path";
import { getCurrentLocale } from "../locale/read";

// The offer page's actions - its lines (when the store left them open), and the step into checkout

async function offerCartId(): Promise<string | null> {
  return (await getOfferSession())?.cartId ?? null;
}

// Checkout works on the offer from here until the shopper goes back to their own cart or places the order
export async function continueOfferCheckout(formData: FormData): Promise<void> {
  const id = String(formData.get("cartId") ?? "");
  const cart = id ? await getCheckoutCart(id) : null;
  if (!cart?.offer) redirect(await withLocalePrefix("/cart/offer?state=invalid", await getCurrentLocale()));
  const store = await cookies();
  store.delete(OFFER_CHECKOUT_COOKIE); // retire the browser-wide checkout selector
  redirect(await withLocalePrefix(checkoutHref(cart.id), await getCurrentLocale()));
}

export async function updateOfferLine(formData: FormData): Promise<void> {
  const cartId = await offerCartId();
  const lineId = String(formData.get("lineId") ?? "");
  const quantity = Number(formData.get("quantity"));
  if (!cartId || !lineId || !(quantity > 0)) return;
  try {
    await gqlFetch(UpdateCartLineDocument, { cartId, lineId, quantity });
  } catch {
    // A locked offer or a bound the product sets - the page re-renders with what the cart really holds
  }
  refresh();
}

export async function removeOfferLine(formData: FormData): Promise<void> {
  const cartId = await offerCartId();
  const lineId = String(formData.get("lineId") ?? "");
  if (!cartId || !lineId) return;
  try {
    await gqlFetch(RemoveFromCartDocument, { cartId, lineId });
  } catch {
    // As above - the re-render shows the cart as it is
  }
  refresh();
}

// Opens an offer from the account list - its owner needs no link, so only the cart is kept. Checked against the
// account's own list, so a forged id opens nothing
export async function openAccountOffer(formData: FormData): Promise<void> {
  const cartId = String(formData.get("cartId") ?? "");
  const locale = await getCurrentLocale();
  let mine: string[] = [];
  try {
    mine = (await gqlFetch(MyCartOffersDocument)).data.myCartOffers.map((c) => c.id);
  } catch {
    // Signed out or the store is down - fall through to the account page
  }
  if (!mine.includes(cartId)) redirect(await withLocalePrefix("/account/offers", locale));
  const store = await cookies();
  store.set(OFFER_COOKIE, encodeOfferSession({ cartId, token: "" }), offerCookieOptions(7 * 24 * 60 * 60));
  store.delete(OFFER_CHECKOUT_COOKIE);
  redirect(await withLocalePrefix("/cart/offer", locale));
}

// The offer page reports the recipient's first look - a repeat, or an id this browser may not open, changes nothing
export async function markOfferOpened(cartId: string): Promise<void> {
  if (!cartId) return;
  await gqlFetch(MarkCartOfferOpenedDocument, { cartId }).catch(() => undefined);
}

// Once the offer is placed it is history - the shopper's own cart takes over again
export async function forgetOffer(): Promise<void> {
  const store = await cookies();
  store.delete(OFFER_COOKIE);
  store.delete(OFFER_CHECKOUT_COOKIE);
}
