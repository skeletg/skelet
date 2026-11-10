import { describe, expect, it } from "vitest";

import { priceView } from "./product-price";

describe("priceView", () => {
  it("quotes no price for a bundle priced by its selections", () => {
    expect(priceView({ kind: "BUNDLE", price: 0, defaultPrice: 0, compareAtPrice: 4000 })).toEqual({ pricedBySelection: true, sellPrice: 0, compareAt: null, savePct: 0 });
  });

  it("strikes through a higher compare-at and works out the saving", () => {
    expect(priceView({ kind: "SIMPLE", price: 7500, defaultPrice: 9000, compareAtPrice: 10000 })).toEqual({ pricedBySelection: false, sellPrice: 7500, compareAt: 10000, savePct: 25 });
    expect(priceView({ kind: "SIMPLE", price: null, defaultPrice: 5000, compareAtPrice: 4000 })).toMatchObject({ sellPrice: 5000, compareAt: null, savePct: 0 });
  });
});
