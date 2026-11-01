import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));
let inbound: Headers | Error = new Headers();
let jar: Map<string, string> | Error = new Map();
vi.mock("next/headers", () => ({
  headers: async () => {
    if (inbound instanceof Error) throw inbound;
    return inbound;
  },
  cookies: async () => {
    if (jar instanceof Error) throw jar;
    const c = jar;
    return { get: (name: string) => (c.has(name) ? { value: c.get(name) } : undefined) };
  },
}));

const { shopperHeaders } = await import("./shopper-headers");
const { clientIP } = await import("./client-ip");

beforeEach(() => {
  inbound = new Headers();
  jar = new Map();
});

describe("clientIP", () => {
  it("passes the recorded chain on verbatim for the backend to walk, falling back to X-Real-IP", () => {
    expect(clientIP(new Headers({ "x-forwarded-for": "203.0.113.7, 172.18.0.5" }))).toBe("203.0.113.7, 172.18.0.5");
    expect(clientIP(new Headers({ "x-real-ip": "203.0.113.9" }))).toBe("203.0.113.9");
    expect(clientIP(new Headers())).toBe("");
  });
});

describe("shopperHeaders", () => {
  it("names the shopper: their IP, browser, the crawler token, their consent and their anon id", async () => {
    inbound = new Headers({ "x-forwarded-for": "203.0.113.7", "user-agent": "Mozilla/5.0 test", "x-skelet-crawl": "token" });
    jar = new Map([["lens_aid", "visitor-1"], ["storefront_consent", "v2:a1.d0.p0"]]);
    expect(await shopperHeaders()).toEqual({
      "X-Forwarded-For": "203.0.113.7",
      "User-Agent": "Mozilla/5.0 test",
      "X-Skelet-Crawl": "token",
      "X-Consent": "v2:a1.d0.p0",
      "X-Anon-Id": "visitor-1",
    });
  });
  it("forwards an old all/essential choice in the category form", async () => {
    jar = new Map([["storefront_consent", "all"]]);
    expect(await shopperHeaders()).toEqual({ "X-Consent": "v2:a1.d1.p1" });
  });
  it("keeps the anon id back without analytics consent - before a choice, and on the request that withdraws it", async () => {
    jar = new Map([["lens_aid", "visitor-1"]]);
    expect(await shopperHeaders()).toEqual({ "X-Consent": "v2:a0.d0.p0" });
    jar = new Map([["lens_aid", "visitor-1"], ["storefront_consent", "v2:a0.d1.p1"]]);
    expect(await shopperHeaders()).toEqual({ "X-Consent": "v2:a0.d1.p1" });
  });
  it("forwards ad click ids only with ads consent, and an affiliate cookie the proxy kept", async () => {
    jar = new Map([["cje", "c1"], ["sk_gclid", "g1"], ["storefront_consent", "v2:a0.d1.p0"]]);
    expect(await shopperHeaders()).toEqual({ "X-Consent": "v2:a0.d1.p0", "X-Click-Ids": "gclid=g1&cjevent=c1" });
    jar = new Map([["cje", "c1"], ["sk_gclid", "g1"], ["storefront_consent", "v2:a1.d0.p1"]]);
    expect(await shopperHeaders()).toEqual({ "X-Consent": "v2:a1.d0.p1", "X-Click-Ids": "cjevent=c1" });
  });
  it("states that nothing is granted before the visitor chooses", async () => {
    expect(await shopperHeaders()).toEqual({ "X-Consent": "v2:a0.d0.p0" });
  });
  it("forwards nothing outside a request, but never swallows Next's dynamic bailout", async () => {
    inbound = new Error("outside a request scope");
    jar = new Error("outside a request scope");
    expect(await shopperHeaders()).toEqual({});
    inbound = Object.assign(new Error("dynamic"), { digest: "DYNAMIC_SERVER_USAGE" });
    await expect(shopperHeaders()).rejects.toThrow("dynamic");
  });
});
