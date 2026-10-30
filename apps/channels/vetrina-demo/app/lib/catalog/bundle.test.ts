import { describe, expect, it } from "vitest";
import { bundlePrice, pickBounds } from "./bundle";

// What this pins: the headline is what the cart line will charge, and the pick bounds are the cart's
describe("bundle price and pick bounds", () => {
  it("prices each mode the way the cart does", () => {
    expect(bundlePrice({ bundlePricing: "SUM", bundleDiscountPercent: 10, price: 0 }, 3100)).toBe(3100);
    expect(bundlePrice({ bundlePricing: "SUM_MINUS_PERCENT", bundleDiscountPercent: 15, price: 0 }, 1010)).toBe(858);
    expect(bundlePrice({ bundlePricing: "FIXED", bundleDiscountPercent: 0, price: 2500 }, 3100)).toBe(2500);
  });

  it("asks a required option for one pick even with no minimum, and a single-select for exactly one", () => {
    const base = { id: "o", title: "Box", sortOrder: 0, selections: [], minSelections: 0, maxSelections: 3 };
    expect(pickBounds({ ...base, inputType: "CHECKBOX", isRequired: true })).toEqual({ min: 1, max: 3 });
    expect(pickBounds({ ...base, inputType: "CHECKBOX", isRequired: false, minSelections: 2 })).toEqual({ min: 2, max: 3 });
    expect(pickBounds({ ...base, inputType: "RADIO", isRequired: false })).toEqual({ min: 0, max: 1 });
  });
});
