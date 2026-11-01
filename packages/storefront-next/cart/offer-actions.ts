"use server";
import "../setup";

import * as core from "@skelet/storefront-core/cart/offer-actions";

export async function continueOfferCheckout(...args: Parameters<typeof core.continueOfferCheckout>) {
  return core.continueOfferCheckout(...args);
}

export async function updateOfferLine(...args: Parameters<typeof core.updateOfferLine>) {
  return core.updateOfferLine(...args);
}

export async function removeOfferLine(...args: Parameters<typeof core.removeOfferLine>) {
  return core.removeOfferLine(...args);
}

export async function openAccountOffer(...args: Parameters<typeof core.openAccountOffer>) {
  return core.openAccountOffer(...args);
}

export async function markOfferOpened(...args: Parameters<typeof core.markOfferOpened>) {
  return core.markOfferOpened(...args);
}

export async function forgetOffer(...args: Parameters<typeof core.forgetOffer>) {
  return core.forgetOffer(...args);
}
