"use client";

import { useEffect, useRef, useState } from "react";

import { useCartStore } from "@/app/lib/cart/store";

// Debounce for typed input so we don't fire a mutation on every keystroke
const TYPE_DEBOUNCE_MS = 550;

export function CartQuantity({
  lineId,
  quantity,
  size = "md",
}: {
  lineId: string;
  quantity: number;
  size?: "sm" | "md";
}) {
  const { setLineQuantity } = useCartStore();
  // display is what the shopper sees; seeded from the store quantity and re-synced to
  // it whenever the store value changes and we're not mid-edit
  const [display, setDisplay] = useState(quantity);
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);
  const dirty = useRef(false);
  const lastSent = useRef(quantity);

  useEffect(() => {
    if (!dirty.current) {
      setDisplay(quantity);
      lastSent.current = quantity;
    }
  }, [quantity]);

  function commit(next: number) {
    if (next === lastSent.current) return; // no change - don't fire a mutation
    lastSent.current = next;
    dirty.current = true;
    setDisplay(next);
    setLineQuantity(lineId, next);
    queueMicrotask(() => {
      dirty.current = false;
    });
  }

  function step(delta: number) {
    if (debounce.current) clearTimeout(debounce.current);
    commit(Math.max(0, display + delta));
  }

  function onType(raw: string) {
    const n = Number(raw);
    const next = Number.isFinite(n) && n >= 0 ? n : 0;
    setDisplay(next);
    dirty.current = true;
    if (debounce.current) clearTimeout(debounce.current);
    debounce.current = setTimeout(() => commit(next), TYPE_DEBOUNCE_MS);
  }

  const btn =
    size === "sm"
      ? "h-7 w-7 text-sm"
      : "h-8 w-8 text-sm";
  const inputW = size === "sm" ? "w-9" : "w-11";

  return (
    <div className="flex items-center gap-1">
      <button
        type="button"
        aria-label="Decrease quantity"
        onClick={() => step(-1)}
        className={`${btn} rounded-lg border border-border transition-colors hover:bg-muted disabled:opacity-40`}
      >
        −
      </button>
      <input
        type="number"
        min={0}
        step="any"
        inputMode="decimal"
        aria-label="Quantity"
        value={display}
        onChange={(e) => onType(e.target.value)}
        onBlur={() => {
          if (debounce.current) clearTimeout(debounce.current);
          commit(display);
        }}
        className={`${inputW} rounded-lg border border-input bg-card px-1 py-0.5 text-center text-sm tabular-nums outline-none focus:border-ring [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none`}
      />
      <button
        type="button"
        aria-label="Increase quantity"
        onClick={() => step(1)}
        className={`${btn} rounded-lg border border-border transition-colors hover:bg-muted`}
      >
        +
      </button>
    </div>
  );
}
