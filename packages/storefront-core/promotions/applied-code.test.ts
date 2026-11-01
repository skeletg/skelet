import { describe, expect, it } from "vitest";

import { describeAppliedCode, giftSummary } from "./applied-code";
import type { AppliedPromotionCode } from "../cart/types";

function code(over: Partial<AppliedPromotionCode> = {}): AppliedPromotionCode {
  return {
    code: "SAVE10",
    ruleName: "Save 10",
    publicLabel: "10% off",
    subtotalDiscount: 1000,
    freeShipping: false,
    invalidatedReason: "",
    requiresCode: true,
    freeGifts: [],
    ...over,
  };
}

describe("describeAppliedCode", () => {
  it("reports a plain subtotal discount", () => {
    const b = describeAppliedCode(code());
    expect(b.discountCents).toBe(1000);
    expect(b.hasVisibleBenefit).toBe(true);
    expect(b.label).toBe("SAVE10");
    expect(b.description).toBe("10% off");
  });

  // The bug: a free-shipping code has no subtotal discount, and the checkout
  // summary printed it as "-$0.00"
  it("a free-shipping code carries no money line", () => {
    const b = describeAppliedCode(
      code({ subtotalDiscount: 0, freeShipping: true }),
    );
    expect(b.discountCents).toBe(0);
    expect(b.freeShipping).toBe(true);
    expect(b.hasVisibleBenefit).toBe(true);
  });

  it("a gift-only code carries no money line but is still a benefit", () => {
    const b = describeAppliedCode(
      code({
        subtotalDiscount: 0,
        freeGifts: [
          { productId: "p1", sku: "MUG", quantity: 1, inCart: true, href: "/p/mug" },
        ],
      }),
    );
    expect(b.discountCents).toBe(0);
    expect(b.giftsInCart).toHaveLength(1);
    expect(b.hasVisibleBenefit).toBe(true);
  });

  it("a gift still on offer is not yet a benefit", () => {
    const b = describeAppliedCode(
      code({
        subtotalDiscount: 0,
        freeGifts: [
          { productId: "p1", sku: "MUG", quantity: 1, inCart: false, href: "/p/mug" },
        ],
      }),
    );
    expect(b.giftsInCart).toHaveLength(0);
    expect(b.hasVisibleBenefit).toBe(false);
  });

  // An invalidated code keeps none of its effects, whatever the server last computed
  it("an invalidated code zeroes every benefit", () => {
    const b = describeAppliedCode(
      code({
        invalidatedReason: "usage limit reached",
        subtotalDiscount: 1000,
        freeShipping: true,
        freeGifts: [
          { productId: "p1", sku: "MUG", quantity: 1, inCart: true, href: "/p/mug" },
        ],
      }),
    );
    expect(b.discountCents).toBe(0);
    expect(b.freeShipping).toBe(false);
    expect(b.giftsInCart).toHaveLength(0);
    expect(b.hasVisibleBenefit).toBe(false);
    expect(b.invalidatedReason).toBe("usage limit reached");
  });

  it("an auto-apply rule labels by its public label, with no duplicate description", () => {
    const b = describeAppliedCode(code({ code: "", publicLabel: "Spring sale" }));
    expect(b.label).toBe("Spring sale");
    expect(b.description).toBe("");
  });

  it("falls back to the rule name when there is no public label", () => {
    const b = describeAppliedCode(code({ code: "", publicLabel: "" }));
    expect(b.label).toBe("Save 10");
  });
});

describe("giftSummary", () => {
  it("prefixes a quantity only above one", () => {
    expect(
      giftSummary([
        { productId: null, sku: "MUG", quantity: 1, inCart: true, href: "" },
        { productId: null, sku: "TOTE", quantity: 2, inCart: true, href: "" },
      ]),
    ).toBe("MUG, 2× TOTE");
  });
});
