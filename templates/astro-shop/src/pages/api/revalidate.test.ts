import { beforeEach, describe, expect, it, vi } from "vitest";

process.env.STOREFRONT_REVALIDATE_SECRET = "s3cret";
const invalidateData = vi.fn();
vi.mock("@skelet/storefront-astro/data-cache", () => ({ invalidateData }));
const { POST } = await import("./revalidate");

const invalidate = vi.fn();

function call(body: unknown, secret = "s3cret") {
  const request = new Request("http://shop.test/api/revalidate", {
    method: "POST",
    headers: { "x-revalidate-secret": secret, "content-type": "application/json" },
    body: JSON.stringify(body),
  });
  // Only what the handler reads from the context
  return POST({ request, url: new URL(request.url), cache: { invalidate } } as never) as Promise<Response>;
}

describe("POST /api/revalidate", () => {
  beforeEach(() => {
    invalidate.mockClear();
    invalidateData.mockClear();
    vi.unstubAllGlobals();
  });

  it("refuses a wrong secret without touching the cache", async () => {
    const res = await call({ tags: ["catalog:browse"] }, "nope");
    expect(res.status).toBe(401);
    expect(invalidate).not.toHaveBeenCalled();
  });

  it("evicts by tag and by path, normalising the leading slash", async () => {
    const res = await call({ tags: ["product:1", 7], paths: ["casual-t-shirt"] });
    expect(res.status).toBe(200);
    expect(invalidate).toHaveBeenCalledWith({ tags: ["product:1"] });
    expect(invalidate).toHaveBeenCalledWith({ path: "/casual-t-shirt" });
    expect(invalidateData).toHaveBeenCalledWith(["product:1"]);
  });

  it("drops the core's cached reads under the same tags - the channel's on public config", async () => {
    await call({ tags: ["config:public", "catalog:browse"] });
    expect(invalidateData).toHaveBeenCalledWith(["config:public", "catalog:browse"]);
  });

  it("rejects a body that isn't JSON", async () => {
    const request = new Request("http://shop.test/api/revalidate", { method: "POST", headers: { "x-revalidate-secret": "s3cret" }, body: "{" });
    const res = (await POST({ request, url: new URL(request.url), cache: { invalidate } } as never)) as Response;
    expect(res.status).toBe(400);
  });

  it("re-renders warm paths on its own origin and counts failures", async () => {
    const fetch = vi.fn(async (url: URL) => new Response("", { status: url.pathname === "/gone" ? 404 : 200 }));
    vi.stubGlobal("fetch", fetch);
    const res = await call({ warm: ["casual-t-shirt", "/gone"] });
    expect(await res.json()).toMatchObject({ warmed: 1, failed: 1 });
    expect(fetch.mock.calls.map(([u]) => String(u))).toEqual(["http://shop.test/casual-t-shirt", "http://shop.test/gone"]);
  });
});
