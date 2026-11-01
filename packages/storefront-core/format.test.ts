import { describe, expect, it } from "vitest";

import { currencyDecimals, formatDate, formatPrice, priceCurrency, toMajor } from "./format";

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

  it("writes the digits the channel's language groups them in, the symbol where the store put it", () => {
    expect(formatPrice(123456, { code: "EUR", symbol: "€", symbolPosition: "suffix", decimals: 2 }, "de")).toBe("1.234,56 €");
    expect(formatPrice(123456, "EUR", "de")).toBe("1.234,56\u00a0€");
  });

  it("writes a currency with no symbol configured the locale's way, not as its bare code", () => {
    expect(formatPrice(1500, { code: "USD", symbol: "", decimals: 2 })).toBe("$15.00");
    expect(formatPrice(1500, { code: "USD", symbol: null })).toBe("$15.00");
  });

  it("accepts a CurrencyInfo object", () => {
    expect(formatPrice(500, { code: "USD", symbol: "$", decimals: 2 })).toContain("5");
  });
});

describe("currency precision", () => {
  it("takes ISO 4217 digits from a bare code: zero, two and three", () => {
    expect(currencyDecimals("JPY")).toBe(0);
    expect(currencyDecimals("USD")).toBe(2);
    expect(currencyDecimals("BHD")).toBe(3);
    expect(currencyDecimals("NOPE")).toBe(2);
  });

  it("prefers the configured decimals, falling back to ISO when they are missing", () => {
    expect(currencyDecimals({ code: "USD", decimals: 0 })).toBe(0);
    expect(currencyDecimals({ code: "BHD" })).toBe(3);
  });

  it("converts minor units to major at that precision", () => {
    expect(toMajor(1500, "JPY")).toBe(1500);
    expect(toMajor(1500, "USD")).toBe(15);
    expect(toMajor(1500, "BHD")).toBe(1.5);
    expect(toMajor(1500, { code: "USD", decimals: 3 })).toBe(1.5);
  });

  it("formats a bare code at ISO precision", () => {
    expect(formatPrice(1500, "JPY")).toBe("¥1,500");
    expect(formatPrice(1500, "USD")).toBe("$15.00");
    expect(formatPrice(1500, "BHD")).toMatch(/1\.500/);
  });

  it("formats configured currencies at their own precision", () => {
    expect(formatPrice(1500, { code: "JPY", symbol: "¥", decimals: 0 })).toBe("¥1,500");
    expect(formatPrice(1500, { code: "BHD", symbol: "BD", symbolPosition: "suffix", decimals: 3 })).toBe("1.500 BD");
  });

  it("uses the configured currency only for prices in that currency", () => {
    const usd = { code: "USD", symbol: "$", decimals: 2 };
    expect(priceCurrency("USD", usd)).toBe(usd);
    expect(priceCurrency("JPY", usd)).toBe("JPY");
    expect(priceCurrency("JPY", null)).toBe("JPY");
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
