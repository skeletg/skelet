import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  gqlFetch: vi.fn(),
  token: null as string | null,
  jar: new Map<string, string>(),
}));
vi.mock("@/app/lib/graphql", () => ({
  gqlFetch: mocks.gqlFetch,
  GraphQLRequestError: class extends Error { constructor(message: string, readonly errors: unknown[]) { super(message); } },
}));
vi.mock("next/headers", () => ({
  cookies: async () => ({
    get: (name: string) => (mocks.jar.has(name) ? { value: mocks.jar.get(name) } : undefined),
    set: (name: string, value: string) => void mocks.jar.set(name, value),
    delete: (name: string) => void mocks.jar.delete(name),
  }),
}));
vi.mock("@/app/lib/auth/session", () => ({ getAccessToken: async () => mocks.token }));
vi.mock("@/app/lib/config/read", () => ({ surfaceCacheOpts: vi.fn(), NAV_SURFACE: "nav", NAV_TAG: "nav" }));
vi.mock("@/app/lib/catalog/read", () => ({ getCartRecommendations: vi.fn() }));

const { addSimpleToCart, addBundleToCart, applyCoupon, setCartLineQuantity, removeCartLine } = await import("./actions");
const { GraphQLRequestError } = await import("@/app/lib/graphql");
const en = (await import("@/messages/en.json")).default;
const { CartByIdDocument, MyCartDocument } = await import("./queries.gql");
const { AddToCartDocument, CartProbeDocument, CreateGuestCartDocument, UpdateCartLineDocument } = await import("./mutations.gql");

const sent = (doc: unknown) => mocks.gqlFetch.mock.calls.filter(([d]) => d === doc).map(([, vars]) => vars);
const add = (sku = "SKU-1") => {
  const form = new FormData();
  form.set("productSku", sku);
  return addSimpleToCart(null, form);
};

beforeEach(() => {
  vi.clearAllMocks();
  mocks.token = null;
  mocks.jar.clear();
  mocks.gqlFetch.mockImplementation(async (doc: unknown) => {
    if (doc === MyCartDocument) return { data: { myCart: { id: "customer-cart" } } };
    if (doc === CartProbeDocument) return { data: { cart: { id: "guest-cart" } } };
    if (doc === CreateGuestCartDocument) return { data: { createGuestCart: { id: "new-guest" } } };
    if (doc === CartByIdDocument) return { data: { cart: { id: "reread" } } };
    return { data: { updateCartLine: { id: "edited" }, removeFromCart: { id: "edited" }, addToCart: { id: "added" } } };
  });
});

describe("signed in without a cart cookie", () => {
  beforeEach(() => {
    mocks.token = "jwt";
  });

  it("edits the session's cart", async () => {
    const res = await setCartLineQuantity("line-1", 3);
    expect(res.cart).toEqual({ id: "edited" });
    expect(sent(UpdateCartLineDocument)).toEqual([{ cartId: "customer-cart", lineId: "line-1", quantity: 3 }]);
    expect(sent(CreateGuestCartDocument)).toEqual([]);
  });

  it("adds to the session's cart without writing a cart cookie", async () => {
    await expect(add()).resolves.toMatchObject({ ok: true });
    expect(sent(AddToCartDocument)).toEqual([{ cartId: "customer-cart", sku: "SKU-1", quantity: 1 }]);
    expect(mocks.jar.size).toBe(0);
  });
});

describe("a signed-in cart that can't be read", () => {
  beforeEach(() => {
    mocks.token = "jwt";
    mocks.jar.set("storefront_cart", "stale-guest");
    const ok = mocks.gqlFetch.getMockImplementation()!;
    mocks.gqlFetch.mockImplementation(async (doc: unknown, vars: unknown) => {
      if (doc === MyCartDocument) throw new Error("backend down");
      return ok(doc, vars);
    });
  });

  it("fails the add instead of using the guest cookie's cart", async () => {
    await expect(add()).resolves.toMatchObject({ ok: false });
    expect(sent(AddToCartDocument)).toEqual([]);
    expect(sent(CreateGuestCartDocument)).toEqual([]);
  });

  it("edits nothing", async () => {
    await expect(removeCartLine("line-1")).resolves.toBeNull();
    expect(mocks.gqlFetch.mock.calls.map(([d]) => d)).toEqual([MyCartDocument]);
  });
});

describe("a guest", () => {
  it("only creates a cart for an addition", async () => {
    await expect(setCartLineQuantity("line-1", 2)).resolves.toEqual({ cart: null });
    const form = new FormData();
    form.set("code", "SAVE10");
    await applyCoupon(undefined, form);
    expect(sent(CreateGuestCartDocument)).toEqual([]);

    await expect(add()).resolves.toMatchObject({ ok: true });
    expect(sent(AddToCartDocument)).toEqual([{ cartId: "new-guest", sku: "SKU-1", quantity: 1 }]);
    expect(mocks.jar.get("storefront_cart")).toBe("new-guest");
  });

  it("doesn't replace a cart the backend failed to probe", async () => {
    mocks.jar.set("storefront_cart", "guest-cart");
    mocks.gqlFetch.mockRejectedValueOnce(new Error("backend down"));
    await expect(add()).resolves.toMatchObject({ ok: false });
    expect(sent(CreateGuestCartDocument)).toEqual([]);
    expect(mocks.jar.get("storefront_cart")).toBe("guest-cart");
  });
});

describe("a bundle add the backend refuses", () => {
  const addBundle = () => {
    const form = new FormData();
    form.set("productSku", "SET-1");
    form.set("selections", "[]");
    return addBundleToCart(null, form);
  };
  const refuse = (code: string | undefined, message: string) =>
    mocks.gqlFetch.mockImplementation(async (doc: unknown) => {
      if (doc === CreateGuestCartDocument) return { data: { createGuestCart: { id: "new-guest" } } };
      throw new GraphQLRequestError(message, [{ message, extensions: code ? { code } : {} }]);
    });

  it.each([
    ["BUNDLE_SELECTION_INVALID", "bundleOutdated"],
    ["BUNDLE_SELECTION_CARDINALITY", "bundleSelectionCount"],
    ["BUNDLE_VARIANT_MISSING", "bundleVariantMissing"],
    ["PRODUCT_NOT_PUBLISHED", "productNotSold"],
  ])("says %s in the shopper's words, never the ids", async (code, key) => {
    refuse(code, "add to cart: bundle selection not on this option: 01a18981-5a5b-7d4a-9ed7-08fe87d64256");
    const res = await addBundle();
    expect(res).toMatchObject({ ok: false, error: key, code });
    expect(en.forms.errors).toHaveProperty(key);
  });

  it("keeps an uncoded message", async () => {
    refuse(undefined, "add to cart rejected: sold out in your region");
    await expect(addBundle()).resolves.toMatchObject({ ok: false, error: "sold out in your region" });
  });
});

// What this pins: the size picked for a configurable bundle selection, and a customer-set amount, reach the cart with the picks
describe("a bundle add with a configurable selection", () => {
  it("sends the picked variant along", async () => {
    const { AddBundleToCartDocument } = await import("./mutations.gql");
    mocks.gqlFetch.mockImplementation(async (doc: unknown) => {
      if (doc === CreateGuestCartDocument) return { data: { createGuestCart: { id: "new-guest" } } };
      return { data: { addBundleToCart: { id: "added" } } };
    });
    const form = new FormData();
    form.set("productSku", "SET-1");
    form.set("selections", JSON.stringify([{ optionId: "o1", selectionIds: ["s1"] }]));
    form.set("variants", JSON.stringify([{ selectionId: "s1", variantId: "hoodie-m" }]));
    form.set("quantities", JSON.stringify([{ selectionId: "s2", quantity: 3 }]));
    await addBundleToCart(null, form);
    expect(sent(AddBundleToCartDocument)[0]).toMatchObject({
      input: {
        sku: "SET-1",
        selections: [{ optionId: "o1", selectionIds: ["s1"] }],
        variants: [{ selectionId: "s1", variantId: "hoodie-m" }],
        quantities: [{ selectionId: "s2", quantity: 3 }],
      },
    });
  });
});
