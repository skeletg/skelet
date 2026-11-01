import { afterEach, beforeEach, expect, it, vi } from "vitest";

const { gqlFetch } = vi.hoisted(() => ({ gqlFetch: vi.fn() }));
vi.mock("../graphql", () => ({ gqlFetch }));
vi.mock("../locale/read", () => ({
  getCurrentChannelId: async () => "ch-1",
  getCurrentLocale: async () => "en",
}));
const { surfaceCacheOpts } = await import("./read");

const settings = (rows: Record<string, string>) => ({
  data: { publicConfig: Object.entries(rows).map(([path, value]) => ({ path, value })) },
});

beforeEach(() => {
  gqlFetch.mockReset();
  vi.stubEnv("NODE_ENV", "production");
});
afterEach(() => vi.unstubAllEnvs());

it("caches a surface for its configured TTL and names the surface for the backend", async () => {
  gqlFetch.mockResolvedValue(settings({ "cache/product/enabled": "true", "cache/product/ttl_seconds": "120" }));
  expect(await surfaceCacheOpts("product", { tags: ["product:1"] })).toEqual({
    cache: "force-cache",
    revalidate: 120,
    tags: ["product:1"],
    cacheKeyExtra: ["ch-1", "en"],
    surface: "product",
  });
  expect(gqlFetch).toHaveBeenCalledWith(expect.anything(), {}, expect.objectContaining({ tags: ["config:public"], surface: "config" }));
});

it("keeps a disabled surface purgeable - a one-second TTL with its tags, never no-store", async () => {
  gqlFetch.mockResolvedValue(settings({ "cache/catalog/enabled": "false", "cache/catalog/ttl_seconds": "3600" }));
  expect(await surfaceCacheOpts("catalog", { tags: ["catalog:browse"] })).toMatchObject({
    cache: "force-cache",
    revalidate: 1,
    tags: ["catalog:browse"],
  });
});

it("falls back to the registered defaults when the settings can't be read", async () => {
  gqlFetch.mockRejectedValue(new Error("backend down"));
  expect(await surfaceCacheOpts("nav", { tags: ["nav:all"] })).toMatchObject({ revalidate: 3600, surface: "nav" });
  gqlFetch.mockResolvedValue(settings({ "cache/nav/ttl_seconds": "not a number" }));
  expect(await surfaceCacheOpts("nav", { tags: ["nav:all"] })).toMatchObject({ revalidate: 3600 });
});

it("never caches in development or for a signed-in visitor", async () => {
  vi.stubEnv("NODE_ENV", "development");
  expect(await surfaceCacheOpts("product", { tags: ["product:1"] })).toEqual({ surface: "product" });
  vi.stubEnv("NODE_ENV", "production");
  expect(await surfaceCacheOpts("product", { tags: ["product:1"], token: "jwt" })).toEqual({});
  expect(gqlFetch).not.toHaveBeenCalled();
});
