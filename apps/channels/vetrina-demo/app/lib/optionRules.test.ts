import { describe, expect, it } from "vitest";

import { evaluateOptionRules, type ProductOptionRule } from "./optionRules";
import { visibleOptionPicks } from "./optionVisibility";

// What these pin: the preview applies rules the way the cart will - require beats optional, quantities that disagree
// conflict, and a percentage rounds on the same base as the options

const gift = { id: "gift", visibleIf: null };
const note = { id: "note", visibleIf: null };
const picksFor = (valueIds: Record<string, string[]>) =>
  visibleOptionPicks([gift, note], (id) => valueIds[id] ?? [], () => false).picks;

const whenGift = { rules: [{ option_id: "gift", op: "anyOf", value_ids: ["yes"] }] };
let seq = 0;
const rule = (actions: ProductOptionRule["actions"], label = ""): ProductOptionRule => ({
  id: `rule-${++seq}`,
  label,
  condition: whenGift,
  actions,
});

describe("evaluateOptionRules", () => {
  it("applies nothing until the condition matches", () => {
    const e = evaluateOptionRules([rule([{ type: "require", optionId: "note", priceUnavailable: false }])], picksFor({}), 2000);
    expect(e.isRequired({ id: "note", isRequired: false })).toBe(false);
    expect(e.adjustments).toEqual([]);
  });

  it("requires over optional, and leaves untouched options alone", () => {
    const e = evaluateOptionRules(
      [
        rule([{ type: "optional", optionId: "note", priceUnavailable: false }]),
        rule([{ type: "require", optionId: "note", priceUnavailable: false }]),
        rule([{ type: "optional", optionId: "gift", priceUnavailable: false }]),
      ],
      picksFor({ gift: ["yes"] }),
      2000,
    );
    expect(e.isRequired({ id: "note", isRequired: false })).toBe(true);
    expect(e.isRequired({ id: "gift", isRequired: true })).toBe(false);
    expect(e.isRequired({ id: "other", isRequired: true })).toBe(true);
  });

  it("sums a rule's price actions into one row, percent off the base", () => {
    const e = evaluateOptionRules(
      [
        rule(
          [
            { type: "adjust_price", priceType: "fixed", priceValue: -500, priceUnavailable: false },
            { type: "adjust_price", priceType: "percent", priceValue: 5, priceUnavailable: false },
          ],
          "Gift set",
        ),
      ],
      picksFor({ gift: ["yes"] }),
      1990,
    );
    expect(e.adjustments).toEqual([expect.objectContaining({ label: "Gift set", amount: -500 + 100 })]);
    expect(e.priceUnavailable).toBe(false);
  });

  it("flags a quantity conflict and an amount this store can't charge", () => {
    const e = evaluateOptionRules(
      [
        rule([{ type: "set_quantity", quantity: 2, priceUnavailable: false }]),
        rule([{ type: "set_quantity", quantity: 3, priceUnavailable: false }]),
        rule([{ type: "adjust_price", priceType: "fixed", priceValue: 300, priceUnavailable: true }]),
      ],
      picksFor({ gift: ["yes"] }),
      2000,
    );
    expect(e.qtyConflict).toBe(true);
    expect(e.priceUnavailable).toBe(true);
  });
});
