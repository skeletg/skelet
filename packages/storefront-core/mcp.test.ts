import { afterEach, describe, expect, it, vi } from "vitest";

import { proxyMcp } from "./mcp";
import { useTestRequest } from "./testing";

afterEach(() => vi.unstubAllGlobals());

function capture(res: Response = new Response('{"ok":true}', { headers: { "content-type": "application/json", "set-cookie": "x=1" } })) {
  const seen: { url: string; init: RequestInit }[] = [];
  vi.stubGlobal("fetch", async (url: string, init: RequestInit) => {
    seen.push({ url, init });
    return res;
  });
  return seen;
}

describe("proxyMcp", () => {
  it("sends the agent to this storefront's channel with nothing of the shopper's", async () => {
    useTestRequest({ channelId: async () => "chan-1" });
    const seen = capture();
    const req = new Request("https://shop.example/api/mcp", {
      method: "POST",
      body: '{"jsonrpc":"2.0"}',
      headers: {
        "content-type": "application/json",
        "mcp-protocol-version": "2025-06-18",
        "x-channel-id": "someone-elses",
        cookie: "storefront_cart=abc",
        authorization: "Bearer customer-token",
        "x-forwarded-for": "203.0.113.9",
      },
    });
    const res = await proxyMcp(req, "http://app/api/front/mcp");

    const sent = new Headers(seen[0].init.headers);
    expect(seen[0].url).toBe("http://app/api/front/mcp");
    expect(sent.get("x-channel-id")).toBe("chan-1");
    expect(sent.get("mcp-protocol-version")).toBe("2025-06-18");
    expect(sent.get("x-forwarded-for")).toBe("203.0.113.9");
    expect(sent.get("cookie")).toBeNull();
    expect(sent.get("authorization")).toBeNull();
    expect(seen[0].init.body).toBe('{"jsonrpc":"2.0"}');
    expect(seen[0].init.signal).toBeInstanceOf(AbortSignal);
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(await res.json()).toEqual({ ok: true });
  });

  it("refuses a body the backend would refuse anyway, without holding it", async () => {
    useTestRequest();
    const seen = capture();
    const res = await proxyMcp(new Request("https://shop.example/api/mcp", { method: "POST", body: "x".repeat(64 * 1024 + 1) }), "http://app/api/front/mcp");
    expect(res.status).toBe(413);
    expect(seen).toHaveLength(0);
  });

  it("answers a backend it can't reach with a 502", async () => {
    useTestRequest();
    vi.stubGlobal("fetch", async () => {
      throw new Error("ECONNREFUSED");
    });
    const res = await proxyMcp(new Request("https://shop.example/api/mcp", { method: "POST", body: "{}" }), "http://app/api/front/mcp");
    expect(res.status).toBe(502);
  });
});
