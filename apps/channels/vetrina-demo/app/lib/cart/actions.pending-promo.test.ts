import { beforeEach, expect, it, vi } from "vitest";

// Answers by operation name; AddToCart fails for any SKU in `failing`
const failing = new Set<string>();
const calls: string[] = [];
const gqlFetch = vi.fn(async (doc: { definitions: { name?: { value: string } }[] }, vars: Record<string, unknown>) => {
  const op = doc.definitions[0]?.name?.value ?? "";
  calls.push(op);
  const sku = (vars.sku ?? (vars.input as { sku?: string } | undefined)?.sku) as string | undefined;
  if (sku && failing.has(sku)) throw new Error("out of stock");
  const cart = { id: "cart-1", op };
  if (op === "CartProbe") return { data: { cart: { id: "cart-1" } } };
  if (op === "ApplyCoupon") return { data: { applyCoupon: { ...cart, coupon: vars.code } } };
  return { data: { addToCart: cart, addBundleToCart: cart, addConfiguredToCart: cart, cart } };
});
vi.mock("@/app/lib/graphql", () => ({ gqlFetch, GraphQLRequestError: class extends Error {} }));

const jar = new Map<string, string>();
vi.mock("next/headers", () => ({
  cookies: async () => ({
    get: (name: string) => (jar.has(name) ? { value: jar.get(name) } : undefined),
    set: (name: string, value: string) => jar.set(name, value),
    delete: (name: string) => jar.delete(name),
  }),
}));
vi.mock("@/app/lib/auth/session", () => ({ getAccessToken: async () => null }));
vi.mock("@/app/lib/cart/read", () => ({
  CART_COOKIE: "cart",
  PROMO_PENDING_COOKIE: "storefront_pending_promo",
  getCart: vi.fn(),
  getOwnCartId: async () => jar.get("cart") ?? null,
}));
vi.mock("@/app/lib/catalog/read", () => ({ getCartRecommendations: vi.fn() }));

const { addBundleToCart, addGroupedToCart, addSimpleToCart, addConfiguredToCart, quickOrderAction } = await import("./actions");

beforeEach(() => {
  failing.clear();
  calls.length = 0;
  jar.clear();
  jar.set("cart", "cart-1");
  jar.set("storefront_pending_promo", "WELCOME10");
});

const form = (fields: Record<string, string | string[]>) => {
  const data = new FormData();
  for (const [k, v] of Object.entries(fields)) for (const one of [v].flat()) data.append(k, one);
  return data;
};
const applied = () => calls.filter((c) => c === "ApplyCoupon").length;
const line = (sku: string) => JSON.stringify({ productSku: sku, productName: sku, quantity: 1 });

it.each([
  ["simple", () => addSimpleToCart(null, form({ productSku: "A" }))],
  ["configured", () => addConfiguredToCart(null, form({ productSku: "A", options: "[]" }))],
  ["bundle", () => addBundleToCart(null, form({ productSku: "A", selections: "[]" }))],
  ["grouped", () => addGroupedToCart(null, form({ line: [line("A"), line("B")] }))],
] as const)("applies the held code once after a %s addition", async (_kind, add) => {
  const res = await add();
  expect(res.ok).toBe(true);
  expect(applied()).toBe(1);
  expect(res.cart).toMatchObject({ coupon: "WELCOME10" });
  expect(jar.has("storefront_pending_promo")).toBe(false);
});

it("applies it once to a quick order, even when some rows fail", async () => {
  failing.add("B");
  const res = await quickOrderAction(null, form({ "sku:0": "A", "qty:0": "1", "sku:1": "B", "qty:1": "1", "sku:2": "C", "qty:2": "1" }));
  expect(res.added).toBe(2);
  expect(applied()).toBe(1);
  expect(res.cart).toMatchObject({ coupon: "WELCOME10" });
});

it("applies it to the grouped members that landed before one failed", async () => {
  failing.add("B");
  const res = await addGroupedToCart(null, form({ line: [line("A"), line("B")] }));
  expect(res.ok).toBe(false);
  expect(applied()).toBe(1);
  expect(res.cart).toMatchObject({ coupon: "WELCOME10" });
});

it("holds the code when nothing was added", async () => {
  failing.add("A");
  await addBundleToCart(null, form({ productSku: "A", selections: "[]" }));
  await addGroupedToCart(null, form({ line: [line("A")] }));
  await quickOrderAction(null, form({ "sku:0": "A", "qty:0": "1" }));
  expect(applied()).toBe(0);
  expect(jar.get("storefront_pending_promo")).toBe("WELCOME10");
});
