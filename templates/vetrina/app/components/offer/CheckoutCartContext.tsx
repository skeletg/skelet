"use client";

import { createContext, useContext, type ReactNode } from "react";
import { checkoutHref } from "@/app/lib/checkout/path";

const CheckoutCartContext = createContext({ cartId: "", offerId: undefined as string | undefined });

export function CheckoutCartProvider({ cartId, offerId, children }: { cartId: string; offerId?: string; children: ReactNode }) {
  return <CheckoutCartContext.Provider value={{ cartId, offerId }}>{children}</CheckoutCartContext.Provider>;
}

// Each mutation submits the cart actually rendered, never a cart selected by another tab.
export function CheckoutCartFields() {
  const { cartId, offerId } = useContext(CheckoutCartContext);
  return <><input type="hidden" name="cartId" value={cartId} />{offerId ? <input type="hidden" name="offer" value={offerId} /> : null}</>;
}

export function useCheckoutHref() {
  const { offerId } = useContext(CheckoutCartContext);
  return (extra: Record<string, string> = {}) => checkoutHref(offerId, extra);
}
