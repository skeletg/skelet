"use client";

import { Link } from "@/app/components/LocaleLink";

import { CartQuantity } from "@/app/components/CartQuantity";
import { CartRemovedRow } from "@/app/components/CartRemovedRow";
import { FreeShippingBar } from "@/app/components/FreeShippingBar";
import { ApproxPrice } from "@/app/components/ApproxPrice";
import { CartLineNotice } from "@/app/components/CartLineNotice";
import { CartLineOptionsList } from "@/app/components/CartLineOptionsList";
import { useChannel } from "@/app/components/ChannelProvider";
import { mergePendingRemovals, useCartStore } from "@/app/lib/cart/store";
import type { CartLine } from "@/app/lib/cart/types";
import { displayName } from "@/app/lib/display";
import { formatPrice } from "@/app/lib/format";
import { RecentlyViewedRail } from "@/app/components/RecentlyViewed";
import { Slots } from "@/app/plugins/Slots";
import { CouponForm } from "./coupon-form";
import { SaveForLaterButton, SavedForLater } from "./SavedForLater";
import { CartEstimator } from "./CartEstimator";
import { CartNote } from "./CartNote";
import { mediaThumb } from "@/app/lib/media-thumb";

const FALLBACK_CURRENCY = "USD";

export function CartView({
  freeShipThreshold,
  recommendations,
}: {
  freeShipThreshold: number;
  recommendations: React.ReactNode;
}) {
  const { cart, pendingRemovals, removeLine, clear, lineNotices, cartNotice } =
    useCartStore();
  const channel = useChannel();
  const currency = channel?.currency ?? FALLBACK_CURRENCY;

  const items = cart ? mergePendingRemovals(cart.lines, pendingRemovals) : [];
  const hasLines = !!cart && cart.lines.length > 0;

  if (items.length === 0) {
    // Nothing live and nothing pending-undo - but saved-for-later items may still be
    // parked here, so surface them instead of a bare "empty cart"
    return (
      <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-12">
        <h1 className="text-3xl font-extrabold tracking-tight">Cart</h1>
        <p className="mt-6 text-muted-foreground">Your cart is empty.</p>
        <Link
          href="/products"
          className="mt-6 inline-block text-sm text-foreground underline underline-offset-2 transition-colors hover:text-signal"
        >
          Browse products →
        </Link>
        <SavedForLater />
        <RecentlyViewedRail />
      </main>
    );
  }

  const netSubtotal = cart ? Math.max(0, cart.subtotal - cart.totalDiscount) : 0;

  const renderLine = (line: CartLine) => (
    <li key={line.id} className="py-4">
      <div className="flex items-start gap-4">
        {line.imageUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={mediaThumb(line.imageUrl, 128)}
            alt={displayName(line.productName, line.productSku)}
            className="size-16 shrink-0 rounded-lg border border-border object-cover"
          loading="lazy" decoding="async" />
        ) : (
          <div className="flex size-16 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-[10px] text-muted-foreground">
            No image
          </div>
        )}
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-foreground">
            {displayName(line.productName, line.productSku)}
          </p>
          <p className="text-xs text-muted-foreground">{line.productSku}</p>
          <CartLineNotice line={line} currency={currency} />
          {lineNotices[line.id] ? (
            <p role="alert" className="mt-1 text-xs font-medium text-amber-700">
              {lineNotices[line.id]}
            </p>
          ) : null}
          <p className="mt-1 text-xs text-muted-foreground">
            {formatPrice(line.unitPrice, currency)} each
          </p>

          {line.components.length > 0 && (
            <ul className="mt-2 space-y-0.5 text-xs text-muted-foreground">
              {line.components
                .slice()
                .sort((a, b) => a.sortOrder - b.sortOrder)
                .map((c) => (
                  <li key={c.id}>
                    ↳ {displayName(c.productName, c.productSku)} ({c.productSku}) ×{c.quantity}
                  </li>
                ))}
            </ul>
          )}

          <CartLineOptionsList options={line.options} currency={currency} />
        </div>

        <CartQuantity lineId={line.id} quantity={line.quantity} />

        <div className="w-24 text-right text-sm font-medium tabular-nums">
          {formatPrice(line.totalPrice, currency)}
        </div>

        <div className="flex flex-col items-end gap-1">
          <button
            type="button"
            onClick={() => removeLine(line.id)}
            className="text-xs text-muted-foreground transition-colors hover:text-destructive"
          >
            Remove
          </button>
          {line.components.length === 0 && line.options.length === 0 && (
            <SaveForLaterButton lineId={line.id} />
          )}
        </div>
      </div>
    </li>
  );

  return (
    <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-12">
      <h1 className="text-3xl font-extrabold tracking-tight">Cart</h1>

      {cartNotice ? (
        <p role="alert" className="mt-4 rounded-lg border border-amber-300/60 bg-amber-50 px-4 py-2 text-sm text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">
          {cartNotice}
        </p>
      ) : null}

      {/* Line items as one paper card - rows separate with hairline dividers */}
      <ul className="glass mt-8 divide-y divide-border rounded-xl px-5">
        {items.map((item) =>
          item.type === "removed" ? (
            <li key={`removed-${item.pending.token}`}>
              <CartRemovedRow pending={item.pending} />
            </li>
          ) : (
            renderLine(item.line)
          ),
        )}
      </ul>

      {hasLines && (
        <>
          <FreeShippingBar
            subtotal={cart.subtotal}
            thresholdCents={freeShipThreshold}
            currency={currency}
            className="mt-6"
          />

          <Slots zone="cart:summary" ctx={{ subtotal: netSubtotal, currency }} />

          <CouponForm
            appliedCodes={cart.appliedCodes}
            totalDiscount={cart.totalDiscount}
            currency={currency}
          />

          <div className="mt-6 flex items-center justify-between">
            <button
              type="button"
              onClick={() => clear()}
              className="text-xs text-muted-foreground transition-colors hover:text-destructive"
            >
              Clear cart
            </button>
            <div className="text-right">
              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Subtotal
              </p>
              <p className="text-sm tabular-nums text-muted-foreground">
                {formatPrice(cart.subtotal, currency)}
                <ApproxPrice cents={cart.subtotal} />
              </p>
              {cart.totalDiscount > 0 ? (
                <p className="text-sm tabular-nums text-emerald-700">
                  Discount −{formatPrice(cart.totalDiscount, currency)}
                </p>
              ) : null}
              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Total
              </p>
              <p className="text-2xl font-semibold tabular-nums">
                {formatPrice(netSubtotal, currency)}
              </p>
            </div>
          </div>

          <div className="mt-8 flex flex-wrap items-center justify-end gap-x-6 gap-y-2">
            <Link
              href="/checkout"
              className="inline-flex h-12 items-center justify-center rounded-md bg-primary px-8 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
            >
              Proceed to checkout →
            </Link>
            <Slots zone="cart:actions" />
          </div>

          <CartEstimator currency={currency} />

          <CartNote initial={cart.note} />
        </>
      )}

      <SavedForLater />

      {recommendations}
    </main>
  );
}
