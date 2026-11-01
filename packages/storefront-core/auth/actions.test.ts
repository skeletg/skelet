import { beforeEach, expect, it, vi } from "vitest";
import { useTestRequest } from "../testing";

const mocks = vi.hoisted(() => ({ gqlFetch: vi.fn(), redirect: vi.fn((to: string) => { throw new Error(`redirect:${to}`); }) }));
useTestRequest({ redirect: mocks.redirect, revalidate: vi.fn(), cookies: async () => ({ get: () => undefined, set: vi.fn(), delete: vi.fn() }) });
vi.mock("../revalidate", () => ({ revalidateAccount: vi.fn() }));
vi.mock("../graphql", () => ({ gqlFetch: mocks.gqlFetch, GraphQLRequestError: class extends Error {} }));
vi.mock("../locale/read", () => ({
  getCurrentLocale: async () => "de",
  getCurrentChannelLocales: async () => ({ codes: ["en", "de"], defaultLocale: "en" }),
}));

const { loginAction, deleteAddressAction } = await import("./actions");
const { redeemLoyaltyPointsAction } = await import("../loyalty/actions");

beforeEach(() => {
  vi.clearAllMocks();
  mocks.gqlFetch.mockResolvedValue({ data: { customerLogin: { accessToken: "a", refreshToken: "r" } } });
});

const login = (returnTo?: string) => {
  const data = new FormData();
  data.set("email", "shopper@example.com");
  data.set("password", "correct-horse-battery");
  if (returnTo) data.set("returnTo", returnTo);
  return loginAction(undefined, data);
};

it("sends a German shopper back to their German account after signing in", async () => {
  await expect(login()).rejects.toThrow("redirect:/de/account");
});

it("keeps an already-localized return destination and its query as they are", async () => {
  await expect(login("/de/checkout?step=payment")).rejects.toThrow("redirect:/de/checkout?step=payment");
  await expect(login("/checkout?step=payment")).rejects.toThrow("redirect:/de/checkout?step=payment");
});

it("localizes address and loyalty outcomes with their query flags", async () => {
  await expect(deleteAddressAction(new FormData())).rejects.toThrow("redirect:/de/account/addresses");
  const points = new FormData();
  points.set("points", "100");
  mocks.gqlFetch.mockResolvedValue({ data: {} });
  await expect(redeemLoyaltyPointsAction(points)).rejects.toThrow("redirect:/de/account?done=redeem");
});
