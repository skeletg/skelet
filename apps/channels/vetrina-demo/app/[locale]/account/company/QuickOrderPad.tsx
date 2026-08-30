"use client";

// B2B quick-order pad: paste-in SKU + quantity rows, one submit, per- row error
// reporting. Rows add independently server-side so a typo'd SKU never sinks the batch

import { useActionState, useState } from "react";

import {
  buttonPrimary,
  inputClass,
} from "@/app/components/account/AccountUI";
import {
  quickOrderAction,
  type QuickOrderResult,
} from "@/app/lib/cart/actions";

const MAX_ROWS = 20;

export function QuickOrderPad() {
  const [rowCount, setRowCount] = useState(3);
  const [state, formAction, pending] = useActionState<
    QuickOrderResult | null,
    FormData
  >(quickOrderAction, null);

  return (
    <section className="glass mt-6 rounded-xl p-5">
      <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        Quick order
      </h2>
      <p className="mt-2 text-sm text-muted-foreground">
        Know your SKUs? Add them straight to the cart.
      </p>
      <form action={formAction} className="mt-4 max-w-md space-y-2">
        {Array.from({ length: rowCount }, (_, i) => (
          <div key={i} className="flex gap-2">
            <input
              name={`sku:${i}`}
              placeholder="SKU"
              aria-label={`SKU row ${i + 1}`}
              className={`${inputClass} flex-1 font-mono`}
            />
            <input
              name={`qty:${i}`}
              type="number"
              min={0}
              step="any"
              placeholder="Qty"
              aria-label={`Quantity row ${i + 1}`}
              className={`${inputClass} w-24 text-right`}
            />
          </div>
        ))}
        <div className="flex items-center gap-4 pt-2">
          <button type="submit" disabled={pending} className={buttonPrimary}>
            {pending ? "Adding…" : "Add all to cart"}
          </button>
          {rowCount < MAX_ROWS && (
            <button
              type="button"
              onClick={() => setRowCount((n) => Math.min(MAX_ROWS, n + 3))}
              className="text-sm text-muted-foreground underline-offset-2 hover:underline"
            >
              More rows
            </button>
          )}
        </div>
      </form>
      {state && (
        <div className="mt-4 space-y-1 text-sm">
          {state.added > 0 && (
            <p className="text-emerald-700">
              Added {state.added} item{state.added === 1 ? "" : "s"} to your
              cart.
            </p>
          )}
          {state.errors.map((e, i) => (
            <p key={i} role="alert" className="text-red-700">
              {e.sku ? `${e.sku}: ` : ""}
              {e.message}
            </p>
          ))}
        </div>
      )}
    </section>
  );
}
