import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("./shopper-headers", () => ({
  shopperHeaders: async () => ({ "X-Consent": "v2:a1.d0.p0", "X-Anon-Id": "visitor-1" }),
}));

const { proxyToBackend } = await import("./backend-proxy");

afterEach(() => vi.unstubAllGlobals());

describe("proxyToBackend", () => {
  it("sends the shopper's consent and visitor from the storefront, not what the browser claimed", async () => {
    const fetchMock = vi.fn(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const req = new Request("http://shop.example/t/forms/submit", {
      method: "POST",
      body: "{}",
      headers: { "X-Consent": "v2:a1.d1.p1", "X-Click-Ids": "gclid=forged", "X-Anon-Id": "someone-else" },
    });
    await proxyToBackend(req, "http://backend");
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    const sent = init.headers as Headers;
    expect(url).toBe("http://backend/t/forms/submit");
    expect(sent.get("x-consent")).toBe("v2:a1.d0.p0");
    expect(sent.get("x-anon-id")).toBe("visitor-1");
    expect(sent.get("x-click-ids")).toBeNull();
  });
});
