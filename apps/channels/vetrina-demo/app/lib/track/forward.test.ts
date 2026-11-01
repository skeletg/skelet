import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/app/lib/shopper-headers", () => ({
  shopperHeaders: async () => ({ "X-Consent": "v2:a1.d1.p0", "X-Click-Ids": "gclid=g1" }),
}));
vi.mock("@/app/lib/backend-url", () => ({ BACKEND_INTERNAL_URL: "http://backend" }));

vi.mock("server-only", () => ({}));

const { forwardTrack: POST } = await import("./forward");

afterEach(() => vi.unstubAllGlobals());

const post = (body: string, channel?: string) =>
  POST(new Request("http://shop.example/t/e", { method: "POST", body, headers: channel ? { "x-storefront-channel": channel } : {} }));

describe("/t/e", () => {
  it("forwards the event with the shopper's consent and click ids, as the channel's", async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    const res = await post('{"name":"view_item"}', "ch-1");
    expect(res.status).toBe(204);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("http://backend/api/front/track");
    expect(init.body).toBe('{"name":"view_item"}');
    expect(init.headers).toMatchObject({ "X-Consent": "v2:a1.d1.p0", "X-Click-Ids": "gclid=g1", "X-Channel-Id": "ch-1" });
  });

  it("sends nothing without a channel or with an oversized body, and never fails the shopper", async () => {
    const fetchMock = vi.fn(async () => {
      throw new Error("down");
    });
    vi.stubGlobal("fetch", fetchMock);
    expect((await post("{}")).status).toBe(204);
    expect((await post("x".repeat(40_000), "ch-1")).status).toBe(204);
    expect(fetchMock).not.toHaveBeenCalled();
    expect((await post("{}", "ch-1")).status).toBe(204);
  });
});
