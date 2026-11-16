import { beforeEach, describe, expect, it, vi } from "vitest";
import { memoryJar, useTestRequest } from "../testing";

const mocks = vi.hoisted(() => ({
  gqlFetch: vi.fn(),
  redirect: vi.fn((to: string) => {
    throw new Error(`redirect:${to}`);
  }),
  jar: { current: undefined as unknown },
}));
vi.mock("../graphql", () => ({
  gqlFetch: mocks.gqlFetch,
  GraphQLRequestError: class extends Error {
    errors: { extensions?: { code?: string } }[];
    constructor(code: string) {
      super(code);
      this.errors = [{ extensions: { code } }];
    }
  },
}));
vi.mock("../locale/read", () => ({
  getCurrentLocale: async () => "en",
  getCurrentChannelLocales: async () => ({ codes: ["en", "de"], defaultLocale: "en" }),
}));
vi.mock("./merge-cart", () => ({ mergeGuestCartIfNeeded: async () => true }));

const graphql = await import("../graphql");
const { completeSso } = await import("./sso");
const { startSso } = await import("./sso");

let jar = memoryJar();
beforeEach(() => {
  vi.clearAllMocks();
  jar = memoryJar();
  useTestRequest({
    redirect: mocks.redirect,
    cookies: async () => jar,
    headers: async () => new Headers({ "x-forwarded-host": "shop.example", "x-forwarded-proto": "https" }),
  });
});

describe("startSso", () => {
  it("returns to the store's own address, keeping what only this browser finishes with", async () => {
    mocks.gqlFetch.mockResolvedValue({ data: { customerSsoStart: { authorizeUrl: "https://idp.example/authorize", state: "s1", binding: "b1" } } });
    await expect(startSso({ returnTo: "/checkout" })).rejects.toThrow("redirect:https://idp.example/authorize");
    expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), { redirectUri: "https://shop.example/account/sso" });
    expect(JSON.parse(jar.get("storefront_sso")!.value)).toEqual({ state: "s1", binding: "b1", returnTo: "/checkout", locale: "en" });
    expect(jar.options("storefront_sso")).toMatchObject({ httpOnly: true, sameSite: "lax" });
  });

  it("sends the shopper back to the login page, keeping where they were headed, when the store can't start one", async () => {
    mocks.gqlFetch.mockRejectedValue(new Error("backend down"));
    await expect(startSso({ returnTo: "/checkout" })).rejects.toThrow("redirect:/account/login?sso=failed&returnTo=%2Fcheckout");
    mocks.gqlFetch.mockResolvedValue({ data: { customerSsoStart: { authorizeUrl: "javascript:alert(1)//", state: "s1", binding: "b1" } } });
    await expect(startSso({ returnTo: "//evil.example" })).rejects.toThrow("redirect:/account/login?sso=failed");
  });
});

describe("completeSso", () => {
  const pending = (state = "s1", extra: object = {}) =>
    jar.set("storefront_sso", JSON.stringify({ state, binding: "b1", returnTo: "/checkout", ...extra }));

  it("signs the shopper in and sends them on to where they were headed", async () => {
    pending();
    mocks.gqlFetch.mockResolvedValue({ data: { customerSsoComplete: { accessToken: "a", refreshToken: "r" } } });
    await expect(completeSso({ code: "c1", state: "s1", error: null })).rejects.toThrow("redirect:/checkout");
    expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), { state: "s1", code: "c1", binding: "b1" });
    expect(jar.get("storefront_sso")).toBeUndefined();
    expect(jar.get("storefront_access_token")?.value).toBe("a");
  });

  it("refuses a sign-in another browser started, without asking the backend", async () => {
    pending("mine");
    await expect(completeSso({ code: "c1", state: "theirs", error: null })).rejects.toThrow("redirect:/account/login?sso=failed");
    expect(mocks.gqlFetch).not.toHaveBeenCalled();
  });

  it("says why when the provider or the backend turned it down, keeping where the shopper was headed", async () => {
    const back = "&returnTo=%2Fcheckout";
    pending();
    await expect(completeSso({ code: null, state: "s1", error: "access_denied" })).rejects.toThrow(`redirect:/account/login?sso=cancelled${back}`);
    pending();
    await expect(completeSso({ code: null, state: "s1", error: "server_error" })).rejects.toThrow(`redirect:/account/login?sso=failed${back}`);
    pending();
    mocks.gqlFetch.mockRejectedValue(new (graphql.GraphQLRequestError as unknown as new (c: string) => Error)("SSO_EMAIL_UNVERIFIED"));
    await expect(completeSso({ code: "c1", state: "s1", error: null })).rejects.toThrow(`redirect:/account/login?sso=unverified${back}`);
  });

  it("returns the shopper in the language they started in, and only to this store", async () => {
    mocks.gqlFetch.mockResolvedValue({ data: { customerSsoComplete: { accessToken: "a", refreshToken: "r" } } });
    pending("s1", { locale: "de" });
    await expect(completeSso({ code: "c1", state: "s1", error: null })).rejects.toThrow("redirect:/de/checkout");
    pending("s1", { returnTo: "//evil.example/x" });
    await expect(completeSso({ code: "c1", state: "s1", error: null })).rejects.toThrow("redirect:/account");
  });
});
