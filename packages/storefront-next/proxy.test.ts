import { NextRequest } from "next/server";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

const CHANNEL = "56d051bc-0131-4476-bc0b-121d80290684";
let storefrontProxy: typeof import("./proxy").storefrontProxy;

beforeAll(async () => {
  vi.stubEnv("STOREFRONT_CHANNEL_ID", CHANNEL);
  ({ storefrontProxy } = await import("./proxy"));
});

afterEach(() => vi.unstubAllGlobals());

// The backend's answer for the channel's languages, the one call a page request makes
function backend() {
  const fetchMock = vi.fn(async () =>
    Response.json({
      data: {
        currentChannel: {
          defaultLocale: "en",
          locales: [
            { code: "en", hreflangValue: "en" },
            { code: "de", hreflangValue: "de" },
          ],
        },
      },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const event = { waitUntil: vi.fn() } as never;
const req = (path: string, headers: Record<string, string> = {}) =>
  new NextRequest(`http://shop.test${path}`, { headers: { host: "shop.test", ...headers } });

describe("storefrontProxy", () => {
  it("rewrites a page into its channel and default locale, marked as its own", async () => {
    backend();
    const res = await storefrontProxy(req("/cart"), event);
    expect(new URL(res.headers.get("x-middleware-rewrite")!).pathname).toBe(`/${CHANNEL}/en/cart`);
    expect(res.headers.get("x-middleware-request-x-storefront-rewrite")).toBeTruthy();
    expect(res.headers.get("x-middleware-request-x-storefront-channel")).toBe(CHANNEL);
    // A first visit gets a visitor id
    expect(res.cookies.get("lens_aid")?.value).toBeTruthy();
  });

  it("keeps a locale the path already names", async () => {
    backend();
    const res = await storefrontProxy(req("/de/cart"), event);
    expect(new URL(res.headers.get("x-middleware-rewrite")!).pathname).toBe(`/${CHANNEL}/de/cart`);
  });

  it("lets its own rewrite through the second pass, and refuses the channel tree named directly", async () => {
    backend();
    const first = await storefrontProxy(req("/cart"), event);
    const mark = first.headers.get("x-middleware-request-x-storefront-rewrite")!;
    const again = await storefrontProxy(req(`/${CHANNEL}/en/cart`, { "x-storefront-rewrite": mark }), event);
    expect(again.headers.get("x-middleware-next")).toBe("1");
    expect(again.headers.get("x-middleware-request-x-storefront-rewrite")).toBeNull();
    expect((await storefrontProxy(req(`/${CHANNEL}/en/cart`), event)).status).toBe(404);
    expect((await storefrontProxy(req(`/${CHANNEL}/en/cart`, { "x-storefront-rewrite": "guess" }), event)).status).toBe(404);
  });

  it("holds a ?promo= code for the cart and drops it from the address", async () => {
    backend();
    const res = await storefrontProxy(req("/products?promo= save10 "), event);
    expect(res.status).toBe(307);
    expect(new URL(res.headers.get("location")!).search).toBe("");
    expect(res.cookies.get("storefront_pending_promo")?.value).toBe("SAVE10");
  });

  it("stamps the channel on a route handler without rewriting it or asking the backend", async () => {
    const fetchMock = backend();
    const res = await storefrontProxy(req("/api/revalidate", { "x-storefront-rewrite": "stale" }), event);
    expect(res.headers.get("x-middleware-rewrite")).toBeNull();
    expect(res.headers.get("x-middleware-request-x-storefront-channel")).toBe(CHANNEL);
    expect(res.headers.get("x-middleware-request-x-storefront-rewrite")).toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
