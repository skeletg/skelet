"use client";

import { useEffect, useRef } from "react";

import { useCartUI } from "@/app/components/CartUIProvider";
import { useCartStore } from "@/app/lib/cart/store";
import type { AddToCartResult } from "@/app/lib/cart/actions";

export function AddToCartFeedback({ state }: { state: AddToCartResult | null }) {
  const { open } = useCartUI();
  const { applyServerCart } = useCartStore();
  // Identity of the last state object we reacted to, so we fire once per submit (each
  // action call returns a fresh object)
  const last = useRef<AddToCartResult | null>(null);

  useEffect(() => {
    if (state && state !== last.current) {
      last.current = state;
      // Adopt whatever cart came back - a clean success, or the partial cart from a grouped add that failed mid-batch -
      // so the badge + drawer reflect the items that actually landed
      if (state.cart) applyServerCart(state.cart);
      if (state.ok) open();
    }
  }, [state, open, applyServerCart]);

  if (state && !state.ok && state.error) {
    return (
      <p role="alert" className="mt-2 text-sm text-red-600 dark:text-red-400">
        {state.error}
      </p>
    );
  }
  return null;
}
