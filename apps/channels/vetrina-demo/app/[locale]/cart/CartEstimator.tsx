"use client";

// Pre-checkout shipping + tax estimator on the cart: enter a country (+ optional region/ZIP) and see shipping options,
// estimated tax, and an estimated total before committing to checkout. Read-only - it never mutates the cart

import { useState, useTransition } from "react";

import { cartEstimateAction, type CartEstimate } from "@/app/lib/cart/actions";
import { formatPrice, type CurrencyInfo } from "@/app/lib/format";

export function CartEstimator({
  currency,
  defaultCountry = "US",
}: {
  currency: string | CurrencyInfo;
  defaultCountry?: string;
}) {
  const [country, setCountry] = useState(defaultCountry);
  const [region, setRegion] = useState("");
  const [estimate, setEstimate] = useState<CartEstimate | null>(null);
  const [done, setDone] = useState(false);
  const [pending, start] = useTransition();

  return (
    <section className="glass mt-8 rounded-xl p-5">
      <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        Estimate shipping &amp; tax
      </h2>
      <div className="mt-3 flex flex-wrap items-end gap-2">
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          Country
          <input
            value={country}
            onChange={(e) => setCountry(e.target.value)}
            placeholder="US"
            className="h-9 w-24 rounded-lg border border-input bg-card px-2 text-sm uppercase outline-none focus:border-ring"
          />
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          State / region <span className="text-muted-foreground/70">(optional)</span>
          <input
            value={region}
            onChange={(e) => setRegion(e.target.value)}
            placeholder="CA"
            className="h-9 w-28 rounded-lg border border-input bg-card px-2 text-sm uppercase outline-none focus:border-ring"
          />
        </label>
        <button
          type="button"
          disabled={pending || !country.trim()}
          onClick={() =>
            start(async () => {
              const res = await cartEstimateAction(country, region);
              setEstimate(res);
              setDone(true);
            })
          }
          className="h-9 rounded-md border border-border px-4 text-sm font-semibold transition-colors hover:bg-muted disabled:opacity-50"
        >
          {pending ? "Estimating…" : "Estimate"}
        </button>
      </div>

      {done && !estimate && (
        <p className="mt-3 text-xs text-muted-foreground">
          No estimate available for that destination.
        </p>
      )}
      {estimate && (
        <div className="mt-3 space-y-1 text-sm">
          {estimate.shippingOptions.length === 0 ? (
            <p className="text-amber-700">
              We don&rsquo;t ship to that destination yet.
            </p>
          ) : (
            <ul className="space-y-0.5">
              {estimate.shippingOptions.map((o, i) => (
                <li key={i} className="flex justify-between text-muted-foreground">
                  <span>
                    {o.carrierName} · {o.name}
                  </span>
                  <span className="tabular-nums">
                    {o.priceCents === 0 ? "Free" : formatPrice(o.priceCents, currency)}
                  </span>
                </li>
              ))}
            </ul>
          )}
          <div className="flex justify-between border-t border-border pt-1 text-muted-foreground">
            <span>Estimated tax</span>
            <span className="tabular-nums">{formatPrice(estimate.taxCents, currency)}</span>
          </div>
          <div className="flex justify-between font-medium text-foreground">
            <span>Estimated total</span>
            <span className="tabular-nums">
              {formatPrice(estimate.estimatedTotalCents, currency)}
            </span>
          </div>
          <p className="text-[11px] text-muted-foreground/70">
            Estimate only - final shipping &amp; tax are confirmed at checkout.
          </p>
        </div>
      )}
    </section>
  );
}
