"use client";

import { Link } from "@/app/components/LocaleLink";
import { useActionState } from "react";

import { reorderAction, type ReorderResult } from "@/app/lib/account/actions";

export function ReorderButton({ orderId }: { orderId: string }) {
  const [state, formAction, pending] = useActionState<ReorderResult | null, FormData>(
    reorderAction,
    null,
  );

  return (
    <div>
      <form action={formAction}>
        <input type="hidden" name="orderId" value={orderId} />
        <button
          type="submit"
          disabled={pending}
          className="inline-flex h-10 items-center rounded-md border border-border bg-background px-5 text-sm font-semibold hover:bg-accent disabled:opacity-60"
        >
          {pending ? "Adding to cart…" : "Buy it again"}
        </button>
      </form>

      {state?.ok && (
        <div className="mt-2 text-sm">
          <p>
            Added {state.added} item{state.added === 1 ? "" : "s"} to your cart.{" "}
            <Link href="/cart" className="font-medium underline">
              View cart →
            </Link>
          </p>
          {state.skipped && state.skipped.length > 0 && (
            <ul className="mt-1 list-disc pl-5 text-xs text-muted-foreground">
              {state.skipped.map((s, i) => (
                <li key={i}>
                  {s.productName || s.productSku}: {s.reason}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {state && !state.ok && (
        <p role="alert" className="mt-2 text-sm text-red-700">
          {state.error}
        </p>
      )}
    </div>
  );
}
