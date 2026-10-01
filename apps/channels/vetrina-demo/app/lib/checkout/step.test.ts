import { describe, expect, it } from "vitest";

import { DEFAULT_CHECKOUT_SETTINGS, type CheckoutSettings } from "@/app/lib/checkout/settings";
import {
  activeStep,
  detailsComplete,
  estimatedArrival,
  firstIncompleteStep,
  stepStatus,
  type CheckoutProgress,
  type DetailsState,
} from "@/app/lib/checkout/step";

const fresh: CheckoutProgress = { hasDestination: false, hasShippingMethod: false, detailsComplete: false };

describe("checkout step derivation", () => {
  it("walks a shopper through delivery, details and payment as the cart fills in", () => {
    expect(firstIncompleteStep(fresh)).toBe("delivery");
    // A postcode alone isn't enough - an option has to be chosen for it
    expect(firstIncompleteStep({ ...fresh, hasDestination: true })).toBe("delivery");
    const chosen = { ...fresh, hasDestination: true, hasShippingMethod: true };
    expect(firstIncompleteStep(chosen)).toBe("details");
    expect(firstIncompleteStep({ ...chosen, detailsComplete: true })).toBe("payment");
  });

  it("sends the shopper back to delivery when the chosen option stopped being offered", () => {
    const p = { hasDestination: true, hasShippingMethod: false, detailsComplete: true };
    expect(activeStep(p, "payment")).toBe("delivery");
  });

  it("lets ?step reopen a finished step but never skip ahead", () => {
    const p = { hasDestination: true, hasShippingMethod: true, detailsComplete: false };
    expect(activeStep(p, "delivery")).toBe("delivery");
    expect(activeStep(p, "payment")).toBe("details");
    expect(activeStep(p, "nonsense")).toBe("details");
    expect(activeStep(p, null)).toBe("details");
  });

  it("marks a reopened step's neighbours by what the cart holds", () => {
    const p = { hasDestination: true, hasShippingMethod: true, detailsComplete: true };
    const active = activeStep(p, "delivery");
    expect(stepStatus("delivery", active, p)).toBe("active");
    expect(stepStatus("details", active, p)).toBe("completed");
    expect(stepStatus("payment", active, p)).toBe("pending");
  });
});

describe("detailsComplete", () => {
  const s = DEFAULT_CHECKOUT_SETTINGS;
  const delivery: DetailsState = {
    email: "a@b.c",
    phone: "",
    purchaseType: "PERSONAL",
    company: "",
    businessIds: {},
    businessIdFields: [],
    isPickup: false,
    hasShippingAddress: true,
    billingSameAsShipping: true,
    hasBillingAddress: false,
    pickupContact: null,
  };
  const pickup: DetailsState = {
    ...delivery,
    isPickup: true,
    hasShippingAddress: false,
    billingSameAsShipping: false,
    pickupContact: { someoneElse: false },
  };

  it("needs an address and billing for a delivery", () => {
    expect(detailsComplete(delivery, s)).toBe(true);
    expect(detailsComplete({ ...delivery, hasShippingAddress: false }, s)).toBe(false);
    expect(detailsComplete({ ...delivery, billingSameAsShipping: false }, s)).toBe(false);
    expect(detailsComplete({ ...delivery, email: "" }, s)).toBe(false);
  });

  it("needs only who collects for a pickup - no address", () => {
    expect(detailsComplete(pickup, s)).toBe(true);
    expect(detailsComplete({ ...pickup, pickupContact: null }, s)).toBe(false);
  });

  it("asks for the payer's billing address when someone else collects, or the store wants one", () => {
    const other = { ...pickup, pickupContact: { someoneElse: true } };
    expect(detailsComplete(other, s)).toBe(false);
    expect(detailsComplete({ ...other, hasBillingAddress: true }, s)).toBe(true);
    expect(detailsComplete(pickup, { ...s, pickupRequiresBilling: true })).toBe(false);
  });

  it("follows the phone and business settings", () => {
    const strict: CheckoutSettings = { ...s, phone: "required", business: true };
    expect(detailsComplete(delivery, strict)).toBe(false);
    const withPhone = { ...delivery, phone: "+16502530000" };
    expect(detailsComplete(withPhone, strict)).toBe(true);
    const pib = { key: "pib", label: "PIB", required: true, pattern: "", hint: "" };
    const biz = { ...withPhone, purchaseType: "BUSINESS" as const, company: "Acme", businessIdFields: [pib] };
    expect(detailsComplete(biz, strict)).toBe(false);
    expect(detailsComplete({ ...biz, businessIds: { pib: "123456789" } }, strict)).toBe(true);
    // Business purchases switched off - the stale business fields don't hold the shopper up
    expect(detailsComplete({ ...delivery, purchaseType: "BUSINESS" }, s)).toBe(true);
  });
});

describe("estimatedArrival", () => {
  it("counts business days, skipping the weekend", () => {
    const friday = new Date(2026, 9, 2); // Fri 2 Oct 2026
    expect(estimatedArrival(1, friday).getDate()).toBe(5); // Mon
    expect(estimatedArrival(2, friday).getDate()).toBe(6); // Tue
    const monday = new Date(2026, 9, 5);
    expect(estimatedArrival(3, monday).getDate()).toBe(8); // Thu
  });
});
