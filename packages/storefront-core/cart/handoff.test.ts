import { beforeEach, describe, expect, it, vi } from "vitest";
import { useTestRequest } from "../testing";

const mocks = vi.hoisted(() => ({
  gqlFetch: vi.fn(),
  jar: new Map<string, string>(),
}));
vi.mock("../graphql", () => ({
  gqlFetch: mocks.gqlFetch,
  GraphQLRequestError: class extends Error {
    constructor(
      message: string,
      readonly errors: unknown[],
    ) {
      super(message);
    }
  },
}));
vi.mock("../auth/session", () => ({ getAccessToken: async () => null }));
vi.mock("../config/read", () => ({ surfaceCacheOpts: vi.fn(), NAV_SURFACE: "nav", NAV_TAG: "nav" }));
vi.mock("../catalog/read", () => ({ getCartRecommendations: vi.fn() }));
useTestRequest({
  cookies: async () => ({
    get: (name: string) => (mocks.jar.has(name) ? { value: mocks.jar.get(name) } : undefined),
    set: (name: string, value: string) => void mocks.jar.set(name, value),
    delete: (name: string) => void mocks.jar.delete(name),
  }),
});

const { redeemHandoff } = await import("./handoff");
const { GraphQLRequestError } = await import("../graphql");
const { CART_COOKIE } = await import("./read");

const answer = (guestCart: boolean, id: string) =>
  mocks.gqlFetch.mockResolvedValue({ data: { redeemCartHandoff: { guestCart, cart: { id } } } });

beforeEach(() => {
  vi.clearAllMocks();
  mocks.jar.clear();
});

describe("redeemHandoff", () => {
  it("sends the shopper's own cart and remembers the guest cart the lines landed in", async () => {
    mocks.jar.set(CART_COOKIE, "mine");
    answer(true, "landed");
    expect(await redeemHandoff(" tok ")).toBe("ok");
    expect(mocks.gqlFetch.mock.calls[0][1]).toEqual({ token: "tok", cartId: "mine" });
    expect(mocks.jar.get(CART_COOKIE)).toBe("landed");
  });

  it("leaves the cookie alone when the lines went into a signed-in customer's cart", async () => {
    mocks.jar.set(CART_COOKIE, "guest");
    answer(false, "customer-cart");
    expect(await redeemHandoff("tok")).toBe("ok");
    expect(mocks.jar.get(CART_COOKIE)).toBe("guest");
  });

  it.each([
    ["CART_HANDOFF_EXPIRED", "expired"],
    ["CART_HANDOFF_INVALID", "invalid"],
    ["INTERNAL", "failed"],
  ])("answers %s as %s", async (code, state) => {
    mocks.gqlFetch.mockRejectedValue(new (GraphQLRequestError as unknown as new (m: string, e: unknown[]) => Error)("x", [{ extensions: { code } }]));
    expect(await redeemHandoff("tok")).toBe(state);
    expect(mocks.jar.has(CART_COOKIE)).toBe(false);
  });

  it("doesn't ask the backend about an empty link", async () => {
    expect(await redeemHandoff("  ")).toBe("invalid");
    expect(mocks.gqlFetch).not.toHaveBeenCalled();
  });
});
