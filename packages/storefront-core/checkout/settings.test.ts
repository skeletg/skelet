import { describe, expect, it } from "vitest";

import { DEFAULT_CHECKOUT_SETTINGS, parseCheckoutSettings } from "./settings";

const row = (key: string, value: string) => ({ path: `sales/checkout/${key}`, value });

describe("parseCheckoutSettings", () => {
  it("reads the registry defaults when nothing is set", () => {
    expect(parseCheckoutSettings([])).toEqual(DEFAULT_CHECKOUT_SETTINGS);
  });

  it("reads each switch the admin set", () => {
    const s = parseCheckoutSettings([
      row("phone_field", "required"),
      row("business_purchase", "true"),
      row("tax_id_field", "hidden"),
      row("po_number_field", "true"),
      row("pickup_by_someone_else", "false"),
      row("pickup_requires_billing_address", "true"),
      row("threshold_progress", "false"),
      row("terms_consent", "notice"),
      row("min_order", "2500"),
    ]);
    expect(s).toEqual({
      phone: "required",
      business: true,
      taxId: "hidden",
      poNumber: true,
      pickupSomeoneElse: false,
      pickupRequiresBilling: true,
      thresholdProgress: false,
      termsConsent: "notice",
      minOrderCents: 2500,
    });
  });

  it("falls back on values it doesn't recognise rather than guessing", () => {
    const s = parseCheckoutSettings([row("phone_field", "sometimes"), row("terms_consent", "x"), row("min_order", "-5")]);
    expect(s.phone).toBe(DEFAULT_CHECKOUT_SETTINGS.phone);
    expect(s.termsConsent).toBe(DEFAULT_CHECKOUT_SETTINGS.termsConsent);
    expect(s.minOrderCents).toBe(0);
  });
});
