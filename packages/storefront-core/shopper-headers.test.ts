import { beforeEach, describe, expect, it, vi } from "vitest";
import { useTestRequest } from "./testing";

useTestRequest({ headers: async () => {
    if (inbound instanceof Error) throw inbound;
    return inbound;
  },
  cookies: async () => {
    if (jar instanceof Error) throw jar;
    const c = jar;
    return { get: (name: string) => (c.has(name) ? { value: c.get(name) } : undefined) };
  } });
let inbound: Headers | Error = new Headers();
let jar: Map<string, string> | Error = new Map();

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
  it("names the shopper: their IP, the crawler token and their anon id", async () => {
    inbound = new Headers({ "x-forwarded-for": "203.0.113.7", "x-skelet-crawl": "token" });
    jar = new Map([["lens_aid", "visitor-1"]]);
    expect(await shopperHeaders()).toEqual({ "X-Forwarded-For": "203.0.113.7", "X-Skelet-Crawl": "token", "X-Anon-Id": "visitor-1" });
  });
  it("sends nothing it doesn't have", async () => {
    expect(await shopperHeaders()).toEqual({});
  });
  it("forwards nothing outside a request, but never swallows Next's dynamic bailout", async () => {
    inbound = new Error("outside a request scope");
    jar = new Error("outside a request scope");
    expect(await shopperHeaders()).toEqual({});
    inbound = Object.assign(new Error("dynamic"), { digest: "DYNAMIC_SERVER_USAGE" });
    await expect(shopperHeaders()).rejects.toThrow("dynamic");
  });
});
