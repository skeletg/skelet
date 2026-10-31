"use client";

import { useEffect } from "react";

import { markOfferOpened } from "@skelet/storefront-next/cart/offer-actions";

// Reports the recipient's first look from the browser once the offer shows - mail scanners fetch the link and
// may render the page, but rarely run its scripts, so they don't mark an offer opened
export function MarkOfferOpened({ cartId }: { cartId: string }) {
  useEffect(() => {
    markOfferOpened(cartId).catch(() => undefined);
  }, [cartId]);
  return null;
}
