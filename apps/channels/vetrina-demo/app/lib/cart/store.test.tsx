import { act, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Cart, CartLine } from "@/app/lib/cart/types";
import { renderWithIntl } from "@/app/test/render";

// Each action returns a promise the test settles by hand, so it controls the order responses land in
type Deferred<T> = { promise: Promise<T>; resolve: (v: T) => void };
function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}
const calls = {
  setQty: [] as Deferred<{ cart: Cart | null; error?: string }>[],
  remove: [] as Deferred<Cart | null>[],
  restore: [] as Deferred<{ cart: Cart | null; error?: string }>[],
};
vi.mock("@/app/lib/cart/actions", () => ({
  setCartLineQuantity: () => {
    const d = deferred<{ cart: Cart | null; error?: string }>();
    calls.setQty.push(d);
    return d.promise;
  },
  removeCartLine: () => {
    const d = deferred<Cart | null>();
    calls.remove.push(d);
    return d.promise;
  },
  restoreCartLineAction: () => {
    const d = deferred<{ cart: Cart | null; error?: string }>();
    calls.restore.push(d);
    return d.promise;
  },
  clearCart: async () => null,
}));

const { CartStoreProvider, useCartStore } = await import("@/app/lib/cart/store");

const line = (id: string, unitPrice: number, quantity: number): CartLine => ({
  id,
  productName: id,
  productSku: id,
  unitPrice,
  quantity,
  totalPrice: unitPrice * quantity,
  components: [],
  options: [],
});
const cart = (lines: CartLine[], totalDiscount = 0): Cart => ({
  id: "c1",
  note: "",
  subtotal: lines.reduce((s, l) => s + l.totalPrice, 0),
  totalDiscount,
  appliedCodes: [],
  lines,
  savedItems: [],
});

let store: ReturnType<typeof useCartStore>;
// Hands the live store to the test through a callback - writing the module variable from render is off limits
function Probe({ onStore = (s) => (store = s) }: { onStore?: (s: ReturnType<typeof useCartStore>) => void }) {
  const store = useCartStore();
  onStore(store);
  return (
    <p>
      {store.cart?.lines.map((l) => `${l.id}x${l.quantity}`).join(",") ?? "none"} | subtotal{" "}
      {store.cart?.subtotal} | discount {store.cart?.totalDiscount} | count {store.count}
    </p>
  );
}
const text = () => screen.getByText(/subtotal/).textContent;

describe("cart store", () => {
  beforeEach(() => {
    calls.setQty = [];
    calls.remove = [];
    calls.restore = [];
  });

  it("applies a quantity change at once and scales a proportional discount with the subtotal", () => {
    // 10% off: 2 x 1000 = 2000, discount 200
    renderWithIntl(
      <CartStoreProvider initial={cart([line("a", 1000, 2)], 200)}>
        <Probe />
      </CartStoreProvider>,
    );
    act(() => store.setLineQuantity("a", 3));
    expect(text()).toBe("ax3 | subtotal 3000 | discount 300 | count 3");
  });

  it("lets only the newest response win when two quantity edits race", async () => {
    renderWithIntl(
      <CartStoreProvider initial={cart([line("a", 1000, 1)])}>
        <Probe />
      </CartStoreProvider>,
    );
    act(() => store.setLineQuantity("a", 2));
    act(() => store.setLineQuantity("a", 3));
    // The second request answers first, then the stale first one arrives
    await act(async () => calls.setQty[1].resolve({ cart: cart([line("a", 1000, 3)]) }));
    await act(async () => calls.setQty[0].resolve({ cart: cart([line("a", 1000, 2)]) }));
    expect(text()).toBe("ax3 | subtotal 3000 | discount 0 | count 3");
  });

  it("puts a removed line back in its slot on undo, before the server answers", async () => {
    renderWithIntl(
      <CartStoreProvider initial={cart([line("a", 100, 1), line("b", 200, 1), line("c", 300, 1)])}>
        <Probe />
      </CartStoreProvider>,
    );
    act(() => store.removeLine("b"));
    expect(text()).toBe("ax1,cx1 | subtotal 400 | discount 0 | count 2");
    expect(store.pendingRemovals).toHaveLength(1);

    act(() => store.undoRemove(store.pendingRemovals[0].token));
    expect(text()).toBe("ax1,bx1,cx1 | subtotal 600 | discount 0 | count 3");
    expect(store.pendingRemovals).toHaveLength(0);
    expect(calls.restore).toHaveLength(1);
  });

  it("doesn't let a background hydrate overwrite an edit still in flight", async () => {
    renderWithIntl(
      <CartStoreProvider initial={cart([line("a", 1000, 1)])}>
        <Probe />
      </CartStoreProvider>,
    );
    act(() => store.setLineQuantity("a", 5));
    let adopted = true;
    act(() => {
      adopted = store.hydrate(cart([line("a", 1000, 1)]));
    });
    expect(adopted).toBe(false);
    expect(text()).toBe("ax5 | subtotal 5000 | discount 0 | count 5");
  });
});
