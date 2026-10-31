import { beforeEach, describe, expect, it, vi } from "vitest";

const readPublicStores = vi.fn();
const readStoreProduct = vi.fn();
const gqlFetch = vi.fn();
let enabled = true;
vi.mock("./read", () => ({ readPublicStores: (...a: unknown[]) => readPublicStores(...a), readStoreProduct: (...a: unknown[]) => readStoreProduct(...a) }));
vi.mock("@/app/lib/graphql", () => ({ gqlFetch: (...a: unknown[]) => gqlFetch(...a) }));
vi.mock("@/app/lib/config/read", () => ({ getStoreAvailabilityEnabled: async () => enabled }));

const { findStoresAction, pickStoreProductAction, searchStoreProductsAction } = await import("./actions");

const form = (entries: Record<string, string>) => {
  const data = new FormData();
  for (const [k, v] of Object.entries(entries)) data.set(k, v);
  return data;
};
const productId = "1f8b64d2-96f1-4fbd-8252-f42bd396f9c1";
beforeEach(() => {
  enabled = true;
  readPublicStores.mockReset();
  readStoreProduct.mockReset();
  gqlFetch.mockReset();
  vi.spyOn(console, "error").mockImplementation(() => {});
});

describe("findStoresAction", () => {
  it("asks the backend for one page with the parsed filters", async () => {
    readPublicStores.mockResolvedValue({ enabled: true });
    const result = await findStoresAction(undefined, form({ city: "Belgrade", inStockOnly: "on", productId }));
    expect(readPublicStores).toHaveBeenCalledWith({ city: "Belgrade", inStockOnly: true, productId, limit: 20 });
    expect(result.stores).toEqual({ enabled: true });
    expect(result.query).toEqual({ city: "Belgrade", inStockOnly: true, productId });
  });
  it("never reaches the backend with invalid input", async () => {
    const result = await findStoresAction(undefined, form({ productId: "nope" }));
    expect(readPublicStores).not.toHaveBeenCalled();
    expect(result.status).toBe("error");
  });
  it("turns a backend failure into a form error without stores", async () => {
    readPublicStores.mockRejectedValue(new Error("down"));
    const result = await findStoresAction(undefined, form({}));
    expect(result.stores).toBeUndefined();
    expect(result.error?.[""]).toEqual(["unavailable"]);
  });
});

describe("product actions", () => {
  it("search products only while store availability is on", async () => {
    gqlFetch.mockResolvedValue({ data: { searchProducts: { items: [{ id: productId, name: "Mug", sku: "MUG" }] } } });
    expect((await searchStoreProductsAction(undefined, form({ q: "mug" }))).hits).toHaveLength(1);
    enabled = false;
    const off = await searchStoreProductsAction(undefined, form({ q: "mug" }));
    expect(off.hits).toBeUndefined();
    expect(gqlFetch).toHaveBeenCalledTimes(1);
  });
  it("loads the picked product, or reports it gone", async () => {
    readStoreProduct.mockResolvedValueOnce({ id: productId, kind: "SIMPLE" }).mockResolvedValueOnce(null);
    expect((await pickStoreProductAction(undefined, form({ productId }))).product).toEqual({ id: productId, kind: "SIMPLE" });
    expect((await pickStoreProductAction(undefined, form({ productId }))).product).toBeNull();
  });
});
