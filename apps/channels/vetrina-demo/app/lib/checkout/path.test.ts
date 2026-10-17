import { expect, it } from "vitest";
import { checkoutHref } from "./path";
it("preserves offer selection through checkout steps and notices", () => {
  expect(checkoutHref()).toBe("/checkout");
  expect(checkoutHref("offer-1")).toBe("/checkout?offer=offer-1");
  expect(checkoutHref("offer-1", { step: "delivery", notice: "delivery-changed" })).toBe("/checkout?step=delivery&notice=delivery-changed&offer=offer-1");
});
