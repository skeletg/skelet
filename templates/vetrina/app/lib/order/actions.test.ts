import { beforeEach, expect, it, vi } from "vitest";

class GraphQLRequestError extends Error {
  constructor(public errors: { message: string }[]) {
    super(errors[0]?.message);
  }
}
const mocks = vi.hoisted(() => ({
  gqlFetch: vi.fn(),
  getCustomerOrNull: vi.fn(),
  redirect: vi.fn((to: string) => {
    throw new Error(`redirect ${to}`);
  }),
}));
vi.mock("@/app/lib/graphql", () => ({ gqlFetch: mocks.gqlFetch, GraphQLRequestError }));
vi.mock("next/navigation", () => ({ redirect: mocks.redirect }));
vi.mock("@/app/lib/customer/read", () => ({ getCustomerOrNull: mocks.getCustomerOrNull }));
vi.mock("@/app/lib/locale/read", () => ({ getCurrentLocale: async () => "en" }));
vi.mock("@/app/lib/locale/path", () => ({ withLocalePrefix: async (path: string, locale: string) => `/${locale}${path}` }));
const { trackOrderAction } = await import("./actions");

const form = (fields: Record<string, string> = { number: "ORD-000123", email: "a+b@example.com", postalCode: "SW1A 1AA" }) => {
  const data = new FormData();
  for (const [k, v] of Object.entries(fields)) data.set(k, v);
  return data;
};
const found = (trackOrder: unknown) => mocks.gqlFetch.mockResolvedValue({ data: { trackOrder } });

beforeEach(() => {
  vi.clearAllMocks();
  mocks.getCustomerOrNull.mockResolvedValue(null);
});

it("sends the three fields and opens a guest order on the receipt page with its stored email", async () => {
  found({ orderId: "o1", requiresLogin: false, email: "A+B@example.com" });
  await expect(trackOrderAction(undefined, form())).rejects.toThrow("redirect");
  expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), {
    number: "ORD-000123",
    email: "a+b@example.com",
    postalCode: "SW1A 1AA",
  });
  expect(mocks.redirect).toHaveBeenCalledWith("/en/orders/o1?email=A%2BB%40example.com");
});

it("opens an account's order on the account page for a signed-in shopper", async () => {
  found({ orderId: "o2", requiresLogin: true, email: "" });
  mocks.getCustomerOrNull.mockResolvedValue({ id: "c1" });
  await expect(trackOrderAction(undefined, form())).rejects.toThrow("redirect");
  expect(mocks.redirect).toHaveBeenCalledWith("/en/account/orders/o2");
});

it("sends a signed-out shopper to sign in first, then back to the order", async () => {
  found({ orderId: "o2", requiresLogin: true, email: "" });
  await expect(trackOrderAction(undefined, form())).rejects.toThrow("redirect");
  expect(mocks.redirect).toHaveBeenCalledWith("/en/account/login?returnTo=%2Faccount%2Forders%2Fo2");
});

it("says the order wasn't found when nothing matched", async () => {
  found(null);
  const result = await trackOrderAction(undefined, form());
  expect(result.error?.[""]).toEqual(["orderNotFound"]);
  expect(mocks.redirect).not.toHaveBeenCalled();
});

it("passes the backend's rate-limit message through", async () => {
  mocks.gqlFetch.mockRejectedValue(new GraphQLRequestError([{ message: "too many requests; please try again later" }]));
  const result = await trackOrderAction(undefined, form());
  expect(result.error?.[""]).toEqual(["too many requests; please try again later"]);
});

it("refuses blank and malformed fields without asking the backend", async () => {
  const result = await trackOrderAction(undefined, form({ number: "", email: "nope", postalCode: "" }));
  expect(Object.keys(result.error ?? {}).sort()).toEqual(["email", "number", "postalCode"]);
  expect(mocks.gqlFetch).not.toHaveBeenCalled();
});
