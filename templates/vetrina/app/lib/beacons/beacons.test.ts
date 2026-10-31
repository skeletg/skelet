import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));
let accessToken: string | null = null;
vi.mock("@/app/lib/auth/session", () => ({ getAccessToken: async () => accessToken }));
vi.mock("@/app/lib/locale/read", () => ({ getCurrentChannelId: async () => "channel-1" }));
vi.mock("@/app/lib/shopper-headers", () => ({
  shopperHeaders: async () => ({ "X-Forwarded-For": "203.0.113.7", "X-Anon-Id": "visitor-1" }),
}));
vi.mock("@/app/lib/backend-url", () => ({ BACKEND_INTERNAL_URL: "http://backend:8080" }));

const { postBeacon } = await import("./server");
const { beaconFetch } = await import("./client");
const { BEACON } = await import("./paths");

const fetchMock = vi.fn<typeof fetch>(async () => new Response(null, { status: 204 }));
beforeEach(() => {
  accessToken = null;
  fetchMock.mockClear();
  vi.stubGlobal("fetch", fetchMock);
});
afterEach(() => vi.unstubAllGlobals());
const sent = () => {
  const [url, init] = fetchMock.mock.calls.at(-1) as [string, RequestInit];
  return { url, init, headers: new Headers(init.headers) };
};

describe("postBeacon", () => {
  it("posts straight to the backend on the shopper's behalf", async () => {
    await postBeacon(BEACON.backInStock, { productId: "p", email: "a@b.co" });
    const { url, init, headers } = sent();
    expect(url).toBe("http://backend:8080/t/back-in-stock");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ productId: "p", email: "a@b.co" }));
    expect(headers.get("X-Forwarded-For")).toBe("203.0.113.7");
    expect(headers.get("X-Anon-Id")).toBe("visitor-1");
    expect(headers.get("X-Channel-Id")).toBe("channel-1");
    expect(headers.get("Authorization")).toBeNull();
  });
  it("credits a signed-in shopper with their session as a bearer token", async () => {
    accessToken = "jwt";
    await postBeacon(BEACON.signupSubmit, {});
    expect(sent().headers.get("Authorization")).toBe("Bearer jwt");
  });
});

describe("beaconFetch", () => {
  it("adds the channel the layout stamped on <html>, unless the caller names one", async () => {
    vi.stubGlobal("document", { documentElement: { dataset: { channelId: "from-html" } } });
    await beaconFetch(BEACON.experiments, { headers: { Accept: "application/json" } });
    expect(sent().url).toBe("/t/experiments");
    expect(sent().headers.get("X-Channel-Id")).toBe("from-html");
    expect(sent().headers.get("Accept")).toBe("application/json");
    await beaconFetch(BEACON.config, { channelId: "explicit" });
    expect(sent().headers.get("X-Channel-Id")).toBe("explicit");
  });
});
