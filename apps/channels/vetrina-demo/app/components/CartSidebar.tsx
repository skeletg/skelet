"use client";

import { useEffect, useRef, useState } from "react";

import { Link } from "@/app/components/LocaleLink";

import { cartRecommendationsAction, type CartRecoItem } from "@/app/lib/cart/actions";
import { mergePendingRemovals, useCartStore } from "@/app/lib/cart/store";
import type { CartLine } from "@/app/lib/cart/types";
import { CartQuantity } from "./CartQuantity";
import { CartRemovedRow } from "./CartRemovedRow";
import { displayName } from "@/app/lib/display";
import { formatPrice } from "@/app/lib/format";
import { useCartUI } from "./CartUIProvider";
import { Drawer } from "./Drawer";
import { useChannel } from "./ChannelProvider";
import { ApproxPrice } from "./ApproxPrice";
import { FreeShippingBar } from "./FreeShippingBar";
import { CartLineNotice } from "./CartLineNotice";
import { CartLineOptionsList } from "./CartLineOptionsList";
import { mediaThumb } from "@/app/lib/media-thumb";

// Cart lines don't carry their own currency on the response - every line is assumed to
// be in the active store's currency, enforced by catalog listings
const FALLBACK_CURRENCY = "USD";

export function CartSidebar({
  freeShipThreshold = 0,
}: {
  freeShipThreshold?: number;
}) {
  const { isOpen, open: openCart, close: closeCart } = useCartUI();
  const { cart, count, pendingRemovals, removeLine, lineNotices, cartNotice } =
    useCartStore();
  const channel = useChannel();
  const currency = channel?.currency ?? FALLBACK_CURRENCY;

  const items = cart ? mergePendingRemovals(cart.lines, pendingRemovals) : [];

  const renderLine = (line: CartLine) => (
    <li key={line.id} className="p-4">
      <div className="flex items-start gap-3">
        {line.imageUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={mediaThumb(line.imageUrl, 96)}
            alt={displayName(line.productName, line.productSku)}
            className="size-12 shrink-0 rounded-lg border border-border object-cover"
          loading="lazy" decoding="async" />
        ) : (
          <div className="flex size-12 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-[9px] text-muted-foreground">
            No image
          </div>
        )}
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium leading-tight text-foreground">
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
                    ↳ {displayName(c.productName, c.productSku)} ×{c.quantity}
                  </li>
                ))}
            </ul>
          )}
          <CartLineOptionsList options={line.options} />
        </div>
        <div className="text-right text-sm font-medium tabular-nums">
          {formatPrice(line.totalPrice, currency)}
        </div>
      </div>

      <div className="mt-2 flex items-center justify-between">
        <CartQuantity lineId={line.id} quantity={line.quantity} size="sm" />
        <button
          type="button"
          onClick={() => removeLine(line.id)}
          className="inline-flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-destructive"
        >
          Remove
        </button>
      </div>
    </li>
  );

  const [recos, setRecos] = useState<CartRecoItem[]>([]);
  const recosFetched = useRef(false);
  useEffect(() => {
    if (isOpen && !recosFetched.current && count > 0) {
      recosFetched.current = true;
      cartRecommendationsAction().then(setRecos).catch(() => {});
    }
  }, [isOpen, count]);

  return (
    <>
      <button
        type="button"
        onClick={openCart}
        aria-label={`Open cart (${count} item${count === 1 ? "" : "s"})`}
        className="relative flex h-9 w-9 items-center justify-center rounded-md text-foreground transition hover:bg-muted"
      >
        <ShoppingBagIcon />
        {count > 0 && (
          <span className="absolute -right-1 -top-1 min-w-[1.25rem] rounded-full bg-signal px-1 text-center text-[0.65rem] font-bold leading-5 text-white">
            {count}
          </span>
        )}
      </button>

      <Drawer
        open={isOpen}
        onOpenChange={(o) => (o ? openCart() : closeCart())}
        side="right"
        title="Cart"
        hideTitle
      >
        <header className="flex items-center justify-between border-b border-border px-5 py-4">
          <h2 className="text-base font-semibold tracking-tight">
            Your cart{" "}
            <span className="ml-1 text-sm font-normal text-muted-foreground">
              ({count})
            </span>
          </h2>
        </header>

        <div className="flex-1 overflow-y-auto">
          {cartNotice ? (
            <p role="alert" className="border-b border-amber-300/60 bg-amber-50 px-5 py-2 text-xs text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">
              {cartNotice}
            </p>
          ) : null}
          {items.length === 0 ? (
            <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center">
              <p className="text-sm text-muted-foreground">Your cart is empty.</p>
              <Link
                href="/products"
                onClick={closeCart}
                className="text-sm text-foreground underline underline-offset-2 transition-colors hover:text-signal"
              >
                Browse products →
              </Link>
            </div>
          ) : (
            <ul className="divide-y divide-border">
              {items.map((item) =>
                item.type === "removed" ? (
                  <li key={`removed-${item.pending.token}`}>
                    <CartRemovedRow pending={item.pending} size="sm" />
                  </li>
                ) : (
                  renderLine(item.line)
                ),
              )}
            </ul>
          )}

          {recos.length > 0 && (
            <div className="border-t border-border p-4">
              <p className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                You might also need
              </p>
              <ul className="space-y-2">
                {recos.slice(0, 3).map((r) => (
                  <li key={r.id}>
                    <Link
                      href={r.href}
                      onClick={closeCart}
                      className="flex items-center gap-3 rounded-lg p-1 transition-colors hover:bg-muted"
                    >
                      {r.imageUrl ? (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img
                          src={mediaThumb(r.imageUrl, 80)}
                          alt={r.name}
                          className="size-10 shrink-0 rounded-lg border border-border object-cover"
                        loading="lazy" decoding="async" />
                      ) : (
                        <div className="size-10 shrink-0 rounded-lg border border-border bg-muted" />
                      )}
                      <span className="min-w-0 flex-1 truncate text-sm text-foreground">
                        {r.name}
                      </span>
                      <span className="text-sm tabular-nums text-muted-foreground">
                        {formatPrice(r.priceCents, currency)}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>

        {cart && cart.lines.length > 0 && (
          <footer className="border-t border-border p-5">
            <FreeShippingBar
              subtotal={cart.subtotal}
              thresholdCents={freeShipThreshold}
              currency={currency}
              className="mb-4"
            />
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Subtotal
              </span>
              <span className="text-lg font-semibold tabular-nums">
                {formatPrice(cart.subtotal, currency)}
                <ApproxPrice cents={cart.subtotal} />
              </span>
            </div>
            <div className="mt-4 flex flex-col gap-2">
              <Link
                href="/cart"
                onClick={closeCart}
                className="flex h-10 items-center justify-center rounded-md border border-foreground/25 text-sm font-semibold transition-colors hover:border-foreground"
              >
                View full cart
              </Link>
              <Link
                href="/checkout"
                onClick={closeCart}
                className="flex h-10 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/85"
              >
                Checkout
              </Link>
            </div>
          </footer>
        )}
      </Drawer>
    </>
  );
}

function ShoppingBagIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      className="h-5 w-5"
      aria-hidden="true"
    >
      <path d="M3.75 8.25h16.5l-1.4 12.243A1.125 1.125 0 0 1 17.736 21.5H6.264a1.125 1.125 0 0 1-1.119-1.007L3.75 8.25Z" />
      <path d="M8.25 8.25V6a3.75 3.75 0 1 1 7.5 0v2.25" />
    </svg>
  );
}

