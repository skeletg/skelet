"use client";

import { useActionState } from "react";

import { inputClass } from "@/app/components/account/AccountUI";
import {
  requestReturnAction,
  type ReturnResult,
} from "@/app/lib/account/actions";

export type ReturnableItem = {
  orderLineId: string;
  productName: string;
  productSku: string;
  returnableQty: number;
};

export function ReturnRequestForm({
  orderId,
  items,
}: {
  orderId: string;
  items: ReturnableItem[];
}) {
  const [state, formAction, pending] = useActionState<ReturnResult | null, FormData>(
    requestReturnAction,
    null,
  );

  if (state?.ok) {
    return (
      <div className="rounded-lg border border-emerald-300 bg-emerald-50 p-3 text-sm text-emerald-800">
        Return request <span className="font-mono">{state.rmaNumber}</span>{" "}
        submitted. We&rsquo;ll review it and email you with the next steps.
      </div>
    );
  }

  return (
    <form action={formAction} className="space-y-4">
      <input type="hidden" name="orderId" value={orderId} />
      <ul className="divide-y divide-border border-y border-border">
        {items.map((it) => (
          <li
            key={it.orderLineId}
            className="flex items-center justify-between gap-3 py-3"
          >
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{it.productName}</p>
              <p className="text-xs text-muted-foreground">
                {it.productSku} · up to {it.returnableQty}
              </p>
            </div>
            <input
              type="number"
              name={`qty:${it.orderLineId}`}
              min={0}
              max={it.returnableQty}
              step={1}
              defaultValue={0}
              aria-label={`Return quantity for ${it.productName}`}
              className={`${inputClass} w-20 text-right`}
            />
          </li>
        ))}
      </ul>

      <fieldset className="text-sm">
        <legend className="mb-1">What would you like?</legend>
        <label className="mr-4 inline-flex items-center gap-1.5">
          <input type="radio" name="resolution" value="refund" defaultChecked className="h-3.5 w-3.5" />
          Refund
        </label>
        <label className="inline-flex items-center gap-1.5">
          <input type="radio" name="resolution" value="exchange" className="h-3.5 w-3.5" />
          Exchange (replacement item)
        </label>
      </fieldset>

      <label className="block text-sm">
        Reason (optional)
        <textarea
          name="reason"
          rows={2}
          className={`${inputClass} mt-1 w-full`}
          placeholder="e.g. wrong size, changed my mind"
        />
      </label>

      {state && !state.ok && (
        <p role="alert" className="text-sm text-red-700">
          {state.error}
        </p>
      )}

      <button
        type="submit"
        disabled={pending}
        className="inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-60"
      >
        {pending ? "Submitting…" : "Request return"}
      </button>
    </form>
  );
}
