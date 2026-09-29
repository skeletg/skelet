import { describe, expect, it } from "vitest";

import { deriveQtyBounds } from "@/app/components/qty";

describe("deriveQtyBounds", () => {
  it("defaults to whole units with no floor or ceiling", () => {
    expect(deriveQtyBounds({})).toEqual({
      min: undefined,
      max: undefined,
      step: 1,
      decimals: 0,
      defaultValue: 1,
    });
  });

  it("caps at the tighter of stock and the order maximum, ignoring non-positive caps", () => {
    expect(deriveQtyBounds({ salableQuantity: 4, maxOrderQty: 10 }).max).toBe(4);
    expect(deriveQtyBounds({ salableQuantity: 0, maxOrderQty: 10 }).max).toBe(10);
  });

  it("steps by the smallest unit at the configured precision when no step is set", () => {
    const b = deriveQtyBounds({ quantityDecimals: 2 });
    expect(b.step).toBe(0.01);
    expect(b.decimals).toBe(2);
  });

  it("starts at the order minimum when it is above one step", () => {
    expect(deriveQtyBounds({ minOrderQty: 3 }).defaultValue).toBe(3);
    expect(deriveQtyBounds({ minOrderQty: 0.5, quantityStep: 1 }).defaultValue).toBe(1);
  });
});
