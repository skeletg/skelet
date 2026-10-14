"use client";

import type { ReactNode } from "react";

import { Link } from "@/app/components/LocaleLink";
import { OFFER_CHECKOUT_COOKIE } from "@/app/lib/cart/offer-cookie";

// Checkout works on an offer while its flag cookie is set - going back to the shopper's own cart, or into its
// checkout, clears it first. Not httpOnly, so the browser can drop it without a round trip
export function clearOfferCheckout() {
  document.cookie = `${OFFER_CHECKOUT_COOKIE}=; Max-Age=0; path=/; SameSite=Lax`;
}

export function OwnCartLink({ href, className, children }: { href: string; className?: string; children: ReactNode }) {
  return (
    <Link href={href} className={className} onClick={clearOfferCheckout}>
      {children}
    </Link>
  );
}
