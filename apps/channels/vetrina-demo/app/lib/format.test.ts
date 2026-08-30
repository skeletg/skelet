import { describe, expect, it } from "vitest";

import { formatDate, formatPrice } from "@/app/lib/format";

describe("formatPrice", () => {
  it("formats minor units with a currency code", () => {
    expect(formatPrice(9900, "USD")).toMatch(/99/);
  });

  it("keeps cents precision", () => {
    expect(formatPrice(7901, "USD")).toMatch(/79[.,]01/);
  });

  it("falls back to USD for an empty code", () => {
    expect(() => formatPrice(1000, "")).not.toThrow();
  });

  it("accepts a CurrencyInfo object", () => {
    expect(formatPrice(500, { code: "USD", symbol: "$", decimals: 2 })).toContain("5");
  });
});

describe("formatDate", () => {
  it("renders a long-form date", () => {
    expect(formatDate("2026-07-18T00:00:00Z")).toContain("2026");
  });

  it("returns empty for junk input", () => {
    expect(formatDate("not-a-date")).toBe("");
    expect(formatDate(null)).toBe("");
  });
});
