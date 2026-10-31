import { beforeEach, expect, it, vi } from "vitest";
import { getCheckoutCart } from "./offer";

const mocks = vi.hoisted(() => ({ getCart: vi.fn(), gqlFetch: vi.fn() }));
vi.mock("next/headers", () => ({ cookies: async () => ({ get: (name: string) => ({ value: name === "storefront_checkout_offer" ? "1" : JSON.stringify({ cartId: "offer-other", token: "other-token" }) }) }) }));
vi.mock("./read", () => ({ getCart: mocks.getCart }));
vi.mock("../graphql", () => ({ gqlFetch: mocks.gqlFetch, GraphQLRequestError: class extends Error {} }));
beforeEach(() => { vi.clearAllMocks(); mocks.getCart.mockResolvedValue({ id: "shopper" }); });

it("plain checkout stays on the shopper cart despite another tab's offer cookies", async () => {
  expect(await getCheckoutCart()).toEqual({ id: "shopper" });
  expect(mocks.gqlFetch).not.toHaveBeenCalled();
});

it("loads the requested cart id rather than substituting the latest offer cookie", async () => {
  mocks.gqlFetch.mockResolvedValue({ data: { cart: { id: "rendered-cart" } } });
  expect(await getCheckoutCart("rendered-cart")).toEqual({ id: "rendered-cart" });
  expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), { id: "rendered-cart" });
});

it("refuses an inaccessible old offer without falling back to another cart", async () => {
  mocks.gqlFetch.mockRejectedValue(new Error("forbidden"));
  expect(await getCheckoutCart("old-offer")).toBeNull();
  expect(mocks.getCart).not.toHaveBeenCalled();
});
