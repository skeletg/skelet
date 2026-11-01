import { describe, expect, it } from "vitest";

import { money, paragraphs } from "./format";

describe("money", () => {
  it("uses Intl's currency style when no symbol is configured", () => {
    expect(money(4900, { code: "USD", symbol: "", decimals: 2 })).toBe("$49.00");
  });

  it("honours the backend's symbol, position and decimals", () => {
    expect(money(123456, { code: "RSD", symbol: "din.", symbolPosition: "suffix", decimals: 2 })).toBe("1,234.56 din.");
    expect(money(500, { code: "JPY", symbol: "¥", symbolPosition: "prefix", decimals: 0 })).toBe("¥500");
  });
});

describe("paragraphs", () => {
  it("flattens Lexical JSON to plain paragraphs and drops empty ones", () => {
    const doc = {
      root: {
        children: [
          { type: "paragraph", children: [{ text: "Soft " }, { type: "link", children: [{ text: "cotton" }] }] },
          { type: "paragraph", children: [] },
          { type: "heading", children: [{ text: "Care" }] },
        ],
      },
    };
    expect(paragraphs(doc)).toEqual(["Soft cotton", "Care"]);
  });

  it("returns nothing for a missing description", () => {
    expect(paragraphs(null)).toEqual([]);
  });
});
