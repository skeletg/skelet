"use client";

import { useEffect, useRef } from "react";

import type { Cart } from "@/app/lib/cart/types";
import { useCartStore } from "@/app/lib/cart/store";
import { loadCartAction } from "@/app/lib/cart/actions";
import { useCustomerIdentity } from "@/app/lib/customer/client";

// Cheap structural signature - enough to notice a server-side change without a deep
// compare
function signature(cart: Cart | null): string {
  if (!cart) return "";
  const lines = cart.lines.map((l) => `${l.id}:${l.quantity}:${l.totalPrice}`).join(",");
  const saved = cart.savedItems.map((s) => s.id).join(",");
  return `${cart.id}|${cart.subtotal}|${cart.totalDiscount}|${lines}|${saved}|${cart.note}`;
}

export function CartHydrator({ serverCart }: { serverCart?: Cart | null }) { // omit serverCart to fetch after hydration instead, which is what lets a page be cached
  const { hydrate, applyServerCart } = useCartStore();
  const { identity, resolved } = useCustomerIdentity();
  const lastSig = useRef(serverCart === undefined ? "\u0000unfetched" : signature(serverCart));
  // The identity the store's cart belongs to; null until the first one resolves
  const owner = useRef<string | null>(null);

  const sig = serverCart === undefined ? "\u0000unfetched" : signature(serverCart);
  useEffect(() => {
    if (serverCart === undefined) return;
    if (sig === lastSig.current) return;
    if (hydrate(serverCart)) lastSig.current = sig;
  }, [sig, serverCart, hydrate]);

  useEffect(() => {
    if (serverCart !== undefined) return; // the server already supplied it
    let live = true;
    loadCartAction()
      .then((cart) => {
        if (live) hydrate(cart);
      })
      .catch(() => {
        // a failed fetch leaves the store empty - the badge simply stays at zero
      });
    return () => {
      live = false;
    };
  }, [serverCart, hydrate]);

  // Signing in merges the guest cart into the customer's and signing out leaves it, so the cart the store holds
  // belongs to the previous shopper - adopt the new one over any in-flight edit
  const who = resolved ? (identity?.id ?? "") : null;
  useEffect(() => {
    if (who === null) return;
    const previous = owner.current;
    owner.current = who;
    if (previous === null || previous === who) return;
    let live = true;
    loadCartAction()
      .then((cart) => {
        if (live) applyServerCart(cart);
      })
      .catch(() => {
        // keep what's shown - the next edit or navigation reconciles it
      });
    return () => {
      live = false;
    };
  }, [who, applyServerCart]);

  return null;
}
