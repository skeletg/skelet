import { beforeEach, describe, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({ gqlFetch: vi.fn(), redirect: vi.fn(() => { throw new Error("redirect"); }), refresh: vi.fn(), checkoutCart: vi.fn(), checkout: vi.fn(), settings: vi.fn(), deleteCookie: vi.fn(), methods: vi.fn() }));
vi.mock("@/app/lib/graphql", () => ({ gqlFetch: mocks.gqlFetch, GraphQLRequestError: class extends Error { constructor(message: string, readonly errors: unknown[]) { super(message); } } }));
vi.mock("next/navigation", () => ({ redirect: mocks.redirect, RedirectType: { replace: "replace" } }));
vi.mock("next/cache", () => ({ refresh: mocks.refresh, revalidatePath: vi.fn() }));
vi.mock("next/headers", () => ({ cookies: async () => ({ get: (name: string) => name === "storefront_offer" ? { value: JSON.stringify({ cartId: "offer-2", token: "other" }) } : undefined, delete: mocks.deleteCookie }) }));
vi.mock("@/app/lib/cart/read", () => ({ getCart: async () => ({ id: "cart-1" }), CART_COOKIE: "cart" }));
vi.mock("@/app/lib/cart/offer", () => ({ getCheckoutCart: mocks.checkoutCart }));
vi.mock("@/app/lib/config/read", () => ({ getCheckoutSettings: mocks.settings }));
vi.mock("@/app/lib/countries/read", () => ({ getCountries: vi.fn() }));
vi.mock("@/app/lib/customer/read", () => ({ getCustomerOrNull: vi.fn() }));
vi.mock("@/app/lib/locale/read", () => ({ getCurrentLocale: async () => "en" }));
vi.mock("@/app/lib/locale/path", () => ({ withLocalePrefix: async (path: string, locale: string) => `/${locale}${path}` }));
vi.mock("@/app/lib/checkout/read", () => ({ getCheckoutState: mocks.checkout, checkoutPaymentMethods: mocks.methods }));
const { placeOrderAction, applyCheckoutCouponAction, saveDestinationStep, saveGroupDeliveryAction, saveDeliveryPointAction, saveSplitDeliveryAction } = await import("./actions");
beforeEach(() => { vi.clearAllMocks(); mocks.gqlFetch.mockResolvedValue({ data: {} }); mocks.checkoutCart.mockImplementation(async (id: string) => id === "offer-1" ? { id, offer: { isGuest: true } } : id === "cart-1" ? { id, offer: null } : null); mocks.settings.mockResolvedValue({ termsConsent: "notice" }); mocks.checkout.mockResolvedValue({ email: "guest@example.com", totals: { amountDueCents: 0 } }); });
it.each([
  [saveDestinationStep, { countryCode: "RS", postalCode: "11000" }],
  [saveGroupDeliveryAction, { groupKey: "all", selection: "option::builtin::standard" }],
  [saveDeliveryPointAction, { groupKey: "all", pointId: "point-1" }],
  [saveSplitDeliveryAction, { split: "split" }],
] as const)("keeps delivery open after a successful save", async (action, fields) => {
  const data = new FormData();
  data.set("cartId", "cart-1");
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
  data.set("cartId", "cart-1");
  data.set("groupKey", "group-0");
  data.set("selection", selection);
  await expect(saveGroupDeliveryAction(undefined, data)).rejects.toThrow("redirect");
  expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), { cartId: "cart-1", groupKey: "group-0", ...sent });
});


it("translates an offer payment restriction into a form error", async () => {
  const { GraphQLRequestError } = await import("@/app/lib/graphql");
  mocks.gqlFetch.mockRejectedValue(new GraphQLRequestError("Payment not allowed", [{ message: "Payment not allowed", extensions: { code: "CART_OFFER_PAYMENT_NOT_ALLOWED" } }]));
  const data = new FormData();
  data.set("cartId", "cart-1");
  data.set("countryCode", "RS");
  data.set("postalCode", "11000");
  const result = await saveDestinationStep(undefined, data);
  expect(result.error?.[""]).toEqual(["offerPaymentNotAllowed"]);
  expect(mocks.redirect).not.toHaveBeenCalled();
});


it("pins regular checkout writes to the rendered cart while another tab holds an offer", async () => {
  const data = new FormData();
  data.set("cartId", "cart-1");
  data.set("countryCode", "RS"); data.set("postalCode", "11000");
  await expect(saveDestinationStep(undefined, data)).rejects.toThrow("redirect");
  expect(mocks.checkoutCart).toHaveBeenCalledWith("cart-1");
  expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), { cartId: "cart-1", countryCode: "RS", postalCode: "11000" });
});

it("preserves the offer id on delivery redirects", async () => {
  const data = new FormData();
  data.set("cartId", "offer-1"); data.set("countryCode", "RS"); data.set("postalCode", "11000");
  await expect(saveDestinationStep(undefined, data)).rejects.toThrow("redirect");
  expect(mocks.redirect).toHaveBeenCalledWith("/en/checkout?step=delivery&offer=offer-1", "replace");
});

it.each(["", "forged-id"])("refuses missing or unauthorized checkout context %s without writes", async (id) => {
  const data = new FormData();
  data.set("cartId", id); data.set("countryCode", "RS"); data.set("postalCode", "11000");
  const result = await saveDestinationStep(undefined, data);
  expect(result.error?.[""]).toEqual(["checkoutCartMissing"]);
  expect(mocks.gqlFetch).not.toHaveBeenCalled();
});

it("places the rendered regular cart without clearing another tab's offer session", async () => {
  mocks.gqlFetch.mockResolvedValue({ data: { placeOrder: { id: "order-1", paymentClientSecret: null } } });
  const data = new FormData(); data.set("cartId", "cart-1"); data.set("locale", "en");
  await expect(placeOrderAction(undefined, data)).rejects.toThrow("redirect");
  expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ cartId: "cart-1", savedPaymentMethodId: null }));
  expect(mocks.deleteCookie).not.toHaveBeenCalledWith("storefront_offer");
});

it("applies checkout coupons to the rendered offer, not the shopper cart", async () => {
  const data = new FormData(); data.set("cartId", "offer-1"); data.set("code", "WELCOME");
  await applyCheckoutCouponAction(undefined, data);
  expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), { cartId: "offer-1", code: "WELCOME" });
});


it("finishes one offer without clearing the credential for another offer tab", async () => {
  mocks.gqlFetch.mockResolvedValue({ data: { placeOrder: { id: "order-1", paymentClientSecret: null } } });
  const data = new FormData(); data.set("cartId", "offer-1"); data.set("locale", "en");
  await expect(placeOrderAction(undefined, data)).rejects.toThrow("redirect");
  expect(mocks.gqlFetch).toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ cartId: "offer-1", savedPaymentMethodId: null }));
  expect(mocks.deleteCookie).not.toHaveBeenCalledWith("storefront_offer");
});

describe("placing an order that owes money", () => {
  const PlaceOrder = (doc: unknown) => (doc as { definitions: { name?: { value: string } }[] }).definitions[0]?.name?.value === "PlaceOrder";
  const placed = () => mocks.gqlFetch.mock.calls.some(([doc]) => PlaceOrder(doc));
  const submit = (providerSlug: string) => {
    const data = new FormData(); data.set("cartId", "cart-1"); data.set("locale", "en"); data.set("providerSlug", providerSlug);
    return placeOrderAction(undefined, data);
  };
  beforeEach(() => {
    mocks.checkout.mockResolvedValue({ email: "guest@example.com", totals: { amountDueCents: 500 } });
    mocks.methods.mockResolvedValue([{ providerSlug: "cod", isOffline: true }]);
  });

  it("refuses a method the checkout can no longer pay with, before placing anything", async () => {
    const result = await submit("stripe");
    expect(result.error?.[""]).toEqual(["paymentMethodUnavailable"]);
    expect(placed()).toBe(false);
  });

  it("places with an eligible offline method", async () => {
    mocks.gqlFetch.mockResolvedValue({ data: { placeOrder: { id: "order-1", paymentClientSecret: null } } });
    await expect(submit("cod")).rejects.toThrow("redirect");
    expect(placed()).toBe(true);
    expect(mocks.redirect).toHaveBeenCalledWith("/en/orders/order-1?email=guest%40example.com&just_placed=1");
  });

  it("shows an unexpected payment action as an unpaid order, never finalizing it or celebrating the purchase", async () => {
    mocks.gqlFetch.mockResolvedValue({ data: { placeOrder: { id: "order-1", paymentClientSecret: "pi_secret" } } });
    await expect(submit("cod")).rejects.toThrow("redirect");
    expect(mocks.redirect).toHaveBeenCalledWith("/en/orders/order-1?email=guest%40example.com&payment=incomplete");
    expect(mocks.gqlFetch).toHaveBeenCalledTimes(3); // payment method, note, placeOrder - nothing after
  });
});

it("places a fully covered order without asking for a method", async () => {
  mocks.gqlFetch.mockResolvedValue({ data: { placeOrder: { id: "order-1", paymentClientSecret: null } } });
  const data = new FormData(); data.set("cartId", "cart-1"); data.set("locale", "en");
  await expect(placeOrderAction(undefined, data)).rejects.toThrow("redirect");
  expect(mocks.methods).not.toHaveBeenCalled();
});
