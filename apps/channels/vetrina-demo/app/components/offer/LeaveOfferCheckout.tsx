"use client";

import type { ReactNode } from "react";

import { Link } from "@/app/components/LocaleLink";
import { OFFER_CHECKOUT_COOKIE } from "@/app/lib/cart/offer-cookie";

// Drop the legacy browser-wide checkout marker on cart navigation. Current checkout
// uses explicit offer URLs, so another tab cannot change the cart this link selects.
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
