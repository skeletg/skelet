import type { Cart as CoreCart, CartLine } from "@skelet/storefront-core/cart/types";
import type { Currency } from "../lib/format";
import { Price } from "../components/Price";
import { QuantityInput } from "../components/QuantityInput";
import { Button, ButtonLink, Notice } from "../components/ui";

// What the view reads of the core's cart
type Cart = Pick<CoreCart, "id" | "subtotal" | "totalDiscount"> & {
  lines: Pick<CartLine, "id" | "name" | "sku" | "imageUrl" | "unitPrice" | "quantity" | "totalPrice">[];
};
type Props = { cart: Cart | null; flash?: string; currency: Currency };

export function CartView({ cart, flash, currency }: Props) {
  const lines = cart?.lines ?? [];
  return (
    <>
      <h1 className="mb-8 text-3xl font-semibold tracking-tight sm:text-4xl">Your cart</h1>
      {/* What a background post (enhance.ts) swaps in - the lines, the totals and any message, all together */}
      <div data-cart="">
        {flash && <Notice>{flash}</Notice>}
        {lines.length === 0 || !cart ? (
          <div className="rounded-3xl border border-dashed border-border px-6 py-16 text-center">
            <p className="text-lg font-medium">Your cart is empty</p>
            <p className="mt-2 text-muted-foreground">Find something you'll love in the shop.</p>
            <ButtonLink href="/" className="mt-6">
              Keep shopping
            </ButtonLink>
          </div>
        ) : (
          <div className="grid items-start gap-10 lg:grid-cols-[1fr_360px]">
            <ul className="divide-y divide-border border-y border-border">
              {lines.map((l) => (
                <CartLine key={l.id} line={l} currency={currency} />
              ))}
            </ul>
            <aside className="grid gap-3 rounded-3xl border border-border bg-card p-6 lg:sticky lg:top-24">
              <h2 className="text-lg font-semibold">Summary</h2>
              <p className="flex justify-between">
                <span className="text-muted-foreground">Subtotal</span>
                <Price amount={cart.subtotal} currency={currency} className="font-semibold" />
              </p>
              {cart.totalDiscount > 0 && (
                <p className="flex justify-between text-success">
                  <span>Discount</span>
                  <span>
                    −<Price amount={cart.totalDiscount} currency={currency} />
                  </span>
                </p>
              )}
              <p className="text-sm text-muted-foreground">Shipping and tax are worked out at checkout.</p>
              <ButtonLink href="/checkout" data-astro-prefetch="false" className="mt-2 w-full">
                Checkout
              </ButtonLink>
            </aside>
          </div>
        )}
      </div>
    </>
  );
}

// shortcut: a line's +/- and arrows step by 1 whatever the product's quantity step, fetch the step on the line if fractional goods sell here
function CartLine({ line: l, currency }: { line: Cart["lines"][number]; currency: Currency }) {
  return (
    <li className="grid grid-cols-[72px_1fr_auto] gap-4 py-5 sm:grid-cols-[88px_1fr_auto]" data-line="">
      <div className="aspect-4/5 overflow-hidden rounded-xl bg-muted">
        {l.imageUrl && <img src={l.imageUrl} alt="" width="88" height="110" loading="lazy" className="h-full w-full object-cover" />}
      </div>
      <div className="min-w-0">
        <p className="font-medium">{l.name}</p>
        <p className="mt-0.5 text-sm text-muted-foreground">
          <Price amount={l.unitPrice} currency={currency} /> each
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-4">
          <form method="post" className="flex items-center gap-2" data-enhance="[data-cart]">
            <input type="hidden" name="intent" value="update" />
            <input type="hidden" name="lineId" value={l.id} />
            <QuantityInput id={`qty-${l.id}`} value={l.quantity} label={`Quantity for ${l.name}`} item={l.name} autosubmit />
            <Button variant="ghost" type="submit" className="min-h-11 px-4 text-sm js:hidden">
              Update
            </Button>
          </form>
          <form method="post" data-enhance="[data-cart]">
            <input type="hidden" name="intent" value="remove" />
            <input type="hidden" name="lineId" value={l.id} />
            <button type="submit" className="text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline">
              Remove
            </button>
          </form>
        </div>
      </div>
      <p className="font-semibold">
        <Price amount={l.totalPrice} currency={currency} />
      </p>
    </li>
  );
}
