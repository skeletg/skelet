import { expect, it, vi } from "vitest";

const ttl = vi.hoisted(() => vi.fn<(surface: string) => Promise<number>>(async () => 900));
vi.mock("@skelet/storefront-core/config/read", () => ({ CONFIG_TAG: "config:public", NAV_TAG: "nav:all", surfaceTTL: ttl }));

const { cacheCatalog } = await import("./cache");

it("caches for the surface's time under the page's tags and everything every page shows", async () => {
  const set = vi.fn();
  await cacheCatalog({ set } as never, "product", ["product:p1"]);
  expect(ttl).toHaveBeenCalledWith("product");
  expect(set).toHaveBeenCalledWith({ maxAge: 900, swr: 600, tags: ["product:p1", "nav:all", "config:public"] });
});

it("barely keeps a page whose surface is switched off", async () => {
  const set = vi.fn();
  ttl.mockResolvedValueOnce(1);
  await cacheCatalog({ set } as never, "catalog", ["catalog:browse"]);
  expect(set).toHaveBeenCalledWith(expect.objectContaining({ maxAge: 1, swr: 1 }));
});

it("does nothing without a cache - the Container API in tests", async () => {
  await expect(cacheCatalog(undefined, "catalog", [])).resolves.toBeUndefined();
  expect(ttl).toHaveBeenCalledTimes(2);
});
