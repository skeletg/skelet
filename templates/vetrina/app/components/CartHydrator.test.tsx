import { screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import type { Cart } from "@/app/lib/cart/types";
import { renderWithIntl } from "@/app/test/render";

const mocks = vi.hoisted(() => ({
  identity: { identity: null as { id: string } | null, resolved: false },
  load: vi.fn(),
}));
vi.mock("@/app/lib/customer/client", () => ({ useCustomerIdentity: () => mocks.identity }));
vi.mock("@/app/lib/cart/actions", () => ({ loadCartAction: mocks.load }));

const { CartStoreProvider, useCartStore } = await import("@/app/lib/cart/store");
const { CartHydrator } = await import("./CartHydrator");

const cart = (id: string): Cart => ({ id, note: "", subtotal: 0, totalDiscount: 0, appliedCodes: [], lines: [], savedItems: [] });
function CartId() {
  return <p>cart {useCartStore().cart?.id ?? "none"}</p>;
}
const tree = () => (
  <CartStoreProvider initial={null}>
    <CartHydrator />
    <CartId />
  </CartStoreProvider>
);

it("swaps the guest cart for the customer's merged cart after login, without refetching on first resolve", async () => {
  mocks.load.mockResolvedValue(cart("guest"));
  const view = renderWithIntl(tree());
  await screen.findByText("cart guest");

  mocks.identity = { identity: null, resolved: true };
  view.rerender(tree());
  expect(mocks.load).toHaveBeenCalledTimes(1);

  mocks.load.mockResolvedValue(cart("customer"));
  mocks.identity = { identity: { id: "c1" }, resolved: true };
  view.rerender(tree());
  await screen.findByText("cart customer");
  await waitFor(() => expect(mocks.load).toHaveBeenCalledTimes(2));
});
