import { describe, expect, it } from "vitest";

import type { Requirement } from "./asks";
import { detailsAsks } from "./asks";
import { DEFAULT_CHECKOUT_SETTINGS } from "./settings";
import {
  activeStep,
  businessComplete,
  arrivalLabels,
  firstIncompleteStep,
  progressOf,
  stepsFor,
  stepStatus,
  type CheckoutProgress,
} from "./step";

const req = (scope: string, type: string, status: Requirement["status"], mode: Requirement["mode"] = "REQUIRED"): Requirement => ({
  scope,
  type,
  label: type,
  mode,
  status,
  code: "",
  message: "",
});

const fresh: CheckoutProgress = { needsDelivery: true, deliveryDone: false, detailsDone: false };

describe("checkout step derivation", () => {
  it("walks a shopper through delivery, details and payment as the cart fills in", () => {
    expect(firstIncompleteStep(fresh)).toBe("delivery");
    const delivered = { ...fresh, deliveryDone: true };
    expect(firstIncompleteStep(delivered)).toBe("details");
    expect(firstIncompleteStep({ ...delivered, detailsDone: true })).toBe("payment");
  });

  it("skips delivery for a cart with nothing to ship", () => {
    const digital = { needsDelivery: false, deliveryDone: true, detailsDone: false };
    expect(stepsFor(digital)).toEqual(["details", "payment"]);
    expect(firstIncompleteStep(digital)).toBe("details");
    expect(activeStep(digital, "delivery")).toBe("details");
  });

  it("lets ?step reopen a finished step but never skip ahead", () => {
    const p = { needsDelivery: true, deliveryDone: true, detailsDone: false };
    expect(activeStep(p, "delivery")).toBe("delivery");
    expect(activeStep(p, "payment")).toBe("details");
    expect(activeStep(p, "nonsense")).toBe("details");
    expect(activeStep(p, null)).toBe("details");
  });

  it("marks a reopened step's neighbours by what the cart holds", () => {
    const p = { needsDelivery: true, deliveryDone: true, detailsDone: true };
    const active = activeStep(p, "delivery");
    expect(stepStatus("delivery", active, p)).toBe("active");
    expect(stepStatus("details", active, p)).toBe("completed");
    expect(stepStatus("payment", active, p)).toBe("pending");
  });
});

describe("progressOf", () => {
  const base = { fulfillment: "MIXED", destination: { countryCode: "US" } };

  it("holds the delivery step open while any group still owes a choice, a point, a slot or a block", () => {
    for (const type of ["delivery_option", "point"]) {
      const p = progressOf({ ...base, requirements: [req("k:parcel_locker", type, "MISSING")] }, true);
      expect(p.deliveryDone, type).toBe(false);
    }
    const done = progressOf({ ...base, requirements: [req("k:parcel_locker", "point", "SATISFIED")] }, true);
    expect(done.deliveryDone).toBe(true);
  });

  it("leaves the shopper's own details to the details step", () => {
    const p = progressOf(
      { ...base, requirements: [req("order", "billing_address", "MISSING"), req("k:pickup", "collector", "MISSING")] },
      true,
    );
    expect(p.deliveryDone).toBe(true);
    expect(p.detailsDone).toBe(false);
  });

  it("needs a destination before a physical cart's delivery is done", () => {
    expect(progressOf({ ...base, destination: null, requirements: [] }, true).deliveryDone).toBe(false);
    expect(progressOf({ fulfillment: "NONE", destination: null, requirements: [] }, true).deliveryDone).toBe(true);
  });

  it("keeps an unpaid business purchase in details", () => {
    expect(progressOf({ ...base, requirements: [] }, false).detailsDone).toBe(false);
  });
});

describe("detailsAsks", () => {
  it("reads what the chosen deliveries ask of the shopper", () => {
    const asks = detailsAsks([
      req("order", "shipping_address", "MISSING"),
      req("order", "billing_address", "MISSING", "OPTIONAL"),
      req("order", "phone", "OPTIONAL", "OPTIONAL"),
      req("k:pickup", "collector", "MISSING"),
    ]);
    expect(asks).toEqual({ shipping: true, collector: true, billing: "optional", phone: "optional" });
    expect(detailsAsks([])).toEqual({ shipping: false, collector: false, billing: "hidden", phone: "hidden" });
  });
});

describe("businessComplete", () => {
  const s = { ...DEFAULT_CHECKOUT_SETTINGS, business: true };
  const pib = { key: "pib", label: "PIB", required: true, pattern: "", hint: "" };
  const biz = { purchaseType: "BUSINESS" as const, company: "Acme", businessIds: {}, businessIdFields: [pib] };

  it("needs the company and the identifiers its country requires", () => {
    expect(businessComplete(biz, s)).toBe(false);
    expect(businessComplete({ ...biz, businessIds: { pib: "123456789" } }, s)).toBe(true);
    expect(businessComplete({ ...biz, company: "" }, s)).toBe(false);
    // Business purchases switched off - the stale business fields don't hold the shopper up
    expect(businessComplete(biz, DEFAULT_CHECKOUT_SETTINGS)).toBe(true);
  });
});

describe("arrivalLabels", () => {
  const fmt = (date: Date, options: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat("en-GB", options).format(date);
  const c = (methodCode: string, estimatedArrival: string | null) => ({ optionId: "o1", carrierSlug: "builtin", methodCode, estimatedArrival });

  it("labels each choice with the day the backend names, keyed by choice", () => {
    expect(arrivalLabels([c("express", "2026-10-03"), c("standard", "2026-10-08")], fmt)).toEqual({
      "o1::builtin::express": "Sat 3 Oct",
      "o1::builtin::standard": "Thu 8 Oct",
    });
  });

  it("skips a choice with no estimate or an unreadable date", () => {
    expect(arrivalLabels([c("none", null), c("broken", "soon")], fmt)).toEqual({});
  });

  // A date-only value read as local midnight would move a day back west of UTC - it has to stay the day it names
  it("keeps the calendar day whatever the formatter's own timezone", () => {
    const newYork = (date: Date, options: Intl.DateTimeFormatOptions) =>
      new Intl.DateTimeFormat("en-GB", { ...options, timeZone: options.timeZone ?? "America/New_York" }).format(date);
    expect(arrivalLabels([c("express", "2026-10-03")], newYork)["o1::builtin::express"]).toBe("Sat 3 Oct");
  });
});
