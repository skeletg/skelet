import { beforeEach, describe, expect, it, vi } from "vitest";

import { useTestRequest, memoryJar } from "../testing";

const mocks = vi.hoisted(() => ({ gqlFetch: vi.fn(), token: null as string | null }));
vi.mock("../graphql", async (orig) => ({ ...(await orig<typeof import("../graphql")>()), gqlFetch: mocks.gqlFetch }));
vi.mock("../auth/session", () => ({ getAccessToken: async () => mocks.token }));

const { readCart } = await import("./read");
const { BackendUnavailableError, GraphQLRequestError } = await import("../graphql");

const cart = { id: "c1", lines: [] };
const jar = (cookies: Record<string, string>) => useTestRequest({ cookies: async () => memoryJar(cookies) });

beforeEach(() => {
  mocks.gqlFetch.mockReset();
  mocks.token = null;
});

describe("readCart", () => {
  it("knows a guest with no cookie has none", async () => {
    jar({});
    expect(await readCart()).toEqual({ cart: null, unavailable: false });
    expect(mocks.gqlFetch).not.toHaveBeenCalled();
  });

  it("reads the cart a guest's cookie names, and knows when the backend no longer has it", async () => {
    jar({ storefront_cart: "c1" });
    mocks.gqlFetch.mockResolvedValueOnce({ data: { cart } });
    expect(await readCart()).toEqual({ cart, unavailable: false });
    jar({ storefront_cart: "gone" });
    mocks.gqlFetch.mockResolvedValueOnce({ data: { cart: null } });
    expect(await readCart()).toEqual({ cart: null, unavailable: false });
    jar({ storefront_cart: "gone" });
    mocks.gqlFetch.mockRejectedValueOnce(new GraphQLRequestError("no", [{ message: "no", extensions: { class: "NOT_FOUND" } }]));
    expect(await readCart()).toEqual({ cart: null, unavailable: false });
  });

  it("knows nothing when the read failed", async () => {
    jar({ storefront_cart: "c1" });
    mocks.gqlFetch.mockRejectedValueOnce(new BackendUnavailableError("down"));
    expect(await readCart()).toEqual({ cart: null, unavailable: true });
    mocks.token = "t";
    jar({});
    mocks.gqlFetch.mockRejectedValueOnce(new GraphQLRequestError("x", [{ message: "x", extensions: { class: "INTERNAL" } }]));
    expect(await readCart()).toEqual({ cart: null, unavailable: true });
  });
});
