import { beforeEach, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({ gqlFetch: vi.fn(), redirect: vi.fn(() => { throw new Error("redirect"); }), refresh: vi.fn() }));
vi.mock("@/app/lib/graphql", () => ({ gqlFetch: mocks.gqlFetch, GraphQLRequestError: class extends Error {} }));
vi.mock("next/navigation", () => ({ redirect: mocks.redirect, RedirectType: { replace: "replace" } }));
vi.mock("next/cache", () => ({ refresh: mocks.refresh, revalidatePath: vi.fn() }));
vi.mock("next/headers", () => ({ cookies: vi.fn() }));
vi.mock("@/app/lib/cart/read", () => ({ getCart: async () => ({ id: "cart-1" }), CART_COOKIE: "cart" }));
vi.mock("@/app/lib/config/read", () => ({ getCheckoutSettings: vi.fn() }));
vi.mock("@/app/lib/countries/read", () => ({ getCountries: vi.fn() }));
vi.mock("@/app/lib/customer/read", () => ({ getCustomerOrNull: vi.fn() }));
vi.mock("@/app/lib/locale/read", () => ({ getCurrentLocale: async () => "en" }));
vi.mock("@/app/lib/locale/path", () => ({ withLocalePrefix: async (path: string, locale: string) => `/${locale}${path}` }));
const { saveDestinationStep, saveGroupDeliveryAction, saveDeliveryPointAction, saveSplitDeliveryAction } = await import("./actions");
beforeEach(() => { vi.clearAllMocks(); mocks.gqlFetch.mockResolvedValue({ data: {} }); });
it.each([
  [saveDestinationStep, { countryCode: "RS", postalCode: "11000" }],
  [saveGroupDeliveryAction, { groupKey: "all", selection: "option::builtin::standard" }],
  [saveDeliveryPointAction, { groupKey: "all", pointId: "point-1" }],
  [saveSplitDeliveryAction, { split: "split" }],
] as const)("keeps delivery open after a successful save", async (action, fields) => {
  const data = new FormData();
  for (const [key, value] of Object.entries(fields)) data.set(key, value);
  await expect(action(undefined, data)).rejects.toThrow("redirect");
  expect(mocks.redirect).toHaveBeenCalledWith("/en/checkout?step=delivery", "replace");
  expect(mocks.refresh).not.toHaveBeenCalled();
});

// Two options can offer the same carrier method, so the choice names its option and the backend binds exactly that one
it.each([
  ["lane::builtin::express", { optionId: "lane", carrierSlug: "builtin", methodCode: "express" }],
  ["door::dhl::ups:03", { optionId: "door", carrierSlug: "dhl", methodCode: "ups:03" }],
] as const)("sends the chosen option, carrier and method of %s", async (selection, sent) => {
  const data = new FormData();
  data.set("groupKey", "group-0");
  data.set("selection", selection);
  await expect(saveGroupDeliveryAction(undefined, data)).rejects.toThrow("redirect");
  expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), { cartId: "cart-1", groupKey: "group-0", ...sent });
});
