import { NextRequest } from "next/server";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

const CHANNEL = "56d051bc-0131-4476-bc0b-121d80290684";
let storefrontProxy: typeof import("./proxy").storefrontProxy;

const WARM = "11111111-2222-4333-8444-555555555555";

beforeAll(async () => {
  vi.stubEnv("STOREFRONT_CHANNEL_ID", CHANNEL);
  vi.stubEnv("STOREFRONT_REVALIDATE_SECRET", "s3cret-value");
  ({ storefrontProxy } = await import("./proxy"));
});

afterEach(() => vi.unstubAllGlobals());

// The backend's answer for the channel's languages, the one call a page request makes - and, when given, its answer to a
// session refresh
function backend(refresh?: () => Response) {
  const fetchMock = vi.fn(async (_url: string, init?: RequestInit) =>
    refresh && String(init?.body ?? "").includes("customerRefreshToken") ? refresh() : Response.json({
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

  it("holds nothing for a ?promo= that's no code, and still cleans the address", async () => {
    backend();
    const res = await storefrontProxy(req("/products?promo=two%20words"), event);
    expect(res.status).toBe(307);
    expect(res.cookies.get("storefront_pending_promo")).toBeUndefined();
  });

  it("stamps its own channel over one a client sent, and passes on neither warm header", async () => {
    backend();
    const res = await storefrontProxy(
      req("/cart", { "x-storefront-channel": WARM, "x-storefront-warm-channel": WARM, "x-revalidate-secret": "guess" }),
      event,
    );
    expect(res.headers.get("x-middleware-request-x-storefront-channel")).toBe(CHANNEL);
    expect(res.headers.get("x-middleware-request-x-storefront-warm-channel")).toBeNull();
    expect(res.headers.get("x-middleware-request-x-revalidate-secret")).toBeNull();
  });

  it("warms the channel a request names only beside the revalidate secret, which the render never sees", async () => {
    backend();
    const res = await storefrontProxy(req("/cart", { "x-storefront-warm-channel": WARM, "x-revalidate-secret": "s3cret-value" }), event);
    expect(new URL(res.headers.get("x-middleware-rewrite")!).pathname).toBe(`/${WARM}/en/cart`);
    expect(res.headers.get("x-middleware-request-x-revalidate-secret")).toBeNull();
  });

  it("reports a crawler's page fetch to the backend, naming the crawler's address", async () => {
    const fetchMock = backend();
    const waitUntil = vi.fn();
    await storefrontProxy(req("/cart", { "user-agent": "Googlebot/2.1", "x-forwarded-for": "66.249.66.1" }), { waitUntil } as never);
    expect(waitUntil).toHaveBeenCalledOnce();
    const beacon = fetchMock.mock.calls.find(([url]) => String(url).endsWith("/t/crawl"));
    expect((beacon?.[1]?.headers as Record<string, string>)["X-Forwarded-For"]).toBe("66.249.66.1");
  });
});

describe("session refresh", () => {
  const signedIn = (headers: Record<string, string> = {}) => req("/cart", { cookie: "storefront_refresh_token=old-refresh", ...headers });

  it("rotates a session about to expire - for the browser and for this very render", async () => {
    const fetchMock = backend(() => Response.json({ data: { customerRefreshToken: { accessToken: "new-access", refreshToken: "new-refresh" } } }));
    const res = await storefrontProxy(signedIn({ "x-forwarded-for": "203.0.113.7" }), event);
    expect(res.cookies.get("storefront_access_token")?.value).toBe("new-access");
    expect(res.cookies.get("storefront_refresh_token")?.value).toBe("new-refresh");
    const forwarded = res.headers.get("x-middleware-request-cookie") ?? "";
    expect(forwarded).toContain("storefront_access_token=new-access");
    expect(forwarded).toContain("storefront_refresh_token=new-refresh");
    const call = fetchMock.mock.calls.find(([, init]) => String(init?.body ?? "").includes("customerRefreshToken"));
    expect((call?.[1]?.headers as Record<string, string>)["X-Forwarded-For"]).toBe("203.0.113.7");
  });

  it("ends the session only when the backend says the token is no good", async () => {
    backend(() => Response.json({ errors: [{ message: "invalid refresh token", extensions: { class: "UNAUTHENTICATED" } }] }));
    const res = await storefrontProxy(signedIn(), event);
    expect(res.cookies.get("storefront_refresh_token")?.value).toBe("");
    expect(res.cookies.get("storefront_access_token")?.value).toBe("");
  });

  it.each([
    ["a rate limit", () => Response.json({ errors: [{ message: "slow down", extensions: { class: "TOO_MANY_REQUESTS" } }] })],
    ["an outage reported as an error", () => Response.json({ errors: [{ message: "x", extensions: { class: "UNAVAILABLE" } }] })],
    ["a 502", () => new Response("bad gateway", { status: 502 })],
  ])("keeps the session through %s", async (_label, answer) => {
    backend(answer);
    const res = await storefrontProxy(signedIn(), event);
    expect(res.cookies.get("storefront_refresh_token")).toBeUndefined();
    expect(res.cookies.get("storefront_access_token")).toBeUndefined();
  });
});
