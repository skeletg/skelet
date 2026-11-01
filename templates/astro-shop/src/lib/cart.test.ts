import type { AstroCookies } from "astro";
import { beforeEach, describe, expect, it, vi } from "vitest";

const gql = vi.fn();
vi.mock("./gql", async (orig) => ({ ...(await orig<typeof import("./gql")>()), gql }));

const { bumpBadge, cartAction, forgetCart, rememberCart, setFlash, takeFlash, withCart } = await import("./cart");
const { GqlError } = await import("./gql");

// A Map-backed stand-in for Astro's cookie jar - only the calls cart.ts makes
function jar(initial: Record<string, string> = {}) {
  const store = new Map(Object.entries(initial));
  const opts = new Map<string, unknown>();
  const cookies = {
    get: (k: string) => (store.has(k) ? { value: store.get(k)! } : undefined),
    set: (k: string, v: string, o: unknown) => {
      store.set(k, v);
      opts.set(k, o);
    },
    delete: (k: string) => void store.delete(k),
  } as unknown as AstroCookies;
  return { cookies, store, opts };
}

beforeEach(() => gql.mockReset());

describe("withCart", () => {
  it("uses the existing cart", async () => {
    const { cookies } = jar({ cart_id: "c1" });
    expect(await withCart(cookies, async (id) => id)).toBe("c1");
    expect(gql).not.toHaveBeenCalled();
  });

  it("creates a cart when there is none", async () => {
    gql.mockResolvedValueOnce({ createGuestCart: { id: "new" } });
    expect(await withCart(jar().cookies, async (id) => id)).toBe("new");
  });

  it("replaces a cart the backend no longer has", async () => {
    gql.mockResolvedValueOnce({ cart: null }).mockResolvedValueOnce({ createGuestCart: { id: "new" } });
    const fn = vi.fn(async (id: string) => {
      if (id === "gone") throw new GqlError("cart store mismatch", "CART_STORE_MISMATCH");
      return id;
    });
    expect(await withCart(jar({ cart_id: "gone" }).cookies, fn)).toBe("new");
  });

  it("does not throw the cart away over a bad SKU", async () => {
    gql.mockResolvedValueOnce({ cart: { id: "c1" } });
    const err = new GqlError("product not found", "PRODUCT_NOT_FOUND");
    await expect(withCart(jar({ cart_id: "c1" }).cookies, async () => Promise.reject(err))).rejects.toBe(err);
    expect(gql).toHaveBeenCalledTimes(1);
  });
});

describe("cart cookies", () => {
  it("stores the id httpOnly and the count readable for the badge", () => {
    const { cookies, store, opts } = jar();
    rememberCart(cookies, { id: "c1", lines: [{ quantity: 2 }, { quantity: 1.5 }] });
    expect(store.get("cart_id")).toBe("c1");
    expect(store.get("cart_count")).toBe("3.5");
    expect(opts.get("cart_id")).toMatchObject({ httpOnly: true, sameSite: "lax", path: "/" });
    expect(opts.get("cart_count")).toMatchObject({ httpOnly: false });
  });

  it("forgets both", () => {
    const { cookies, store } = jar({ cart_id: "c1", cart_count: "2" });
    forgetCart(cookies);
    expect(store.size).toBe(0);
  });

  it("flash is read once", () => {
    const { cookies } = jar();
    setFlash(cookies, "Out of stock");
    expect(takeFlash(cookies)).toBe("Out of stock");
    expect(takeFlash(cookies)).toBeUndefined();
  });

  it("bump is short-lived and script-readable", () => {
    const { cookies, store, opts } = jar();
    bumpBadge(cookies);
    expect(store.get("cart_bump")).toBe("1");
    expect(opts.get("cart_bump")).toMatchObject({ httpOnly: false, maxAge: 60 });
  });
});

describe("cartAction", () => {
  const form = (fields: Record<string, string>) => {
    const f = new FormData();
    for (const [k, v] of Object.entries(fields)) f.set(k, v);
    return f;
  };

  it("reads add, update and remove", () => {
    expect(cartAction(form({ intent: "add", sku: "TSHIRT", quantity: "2" }))).toEqual({ intent: "add", sku: "TSHIRT", quantity: 2 });
    expect(cartAction(form({ intent: "update", lineId: "l1", quantity: "3" }))).toEqual({ intent: "update", lineId: "l1", quantity: 3 });
    expect(cartAction(form({ intent: "remove", lineId: "l1" }))).toEqual({ intent: "remove", lineId: "l1" });
  });

  it("treats an update to 0 as a remove", () => {
    expect(cartAction(form({ intent: "update", lineId: "l1", quantity: "0" }))).toEqual({ intent: "remove", lineId: "l1" });
  });

  it("refuses an emptied box, a negative quantity and an unknown intent", () => {
    expect(cartAction(form({ intent: "update", lineId: "l1", quantity: "" }))).toBeNull();
    expect(cartAction(form({ intent: "add", sku: "TSHIRT", quantity: "-1" }))).toBeNull();
    expect(cartAction(form({ intent: "wipe", lineId: "l1" }))).toBeNull();
  });
});
