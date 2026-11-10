import { describe, expect, it } from "vitest";

import { isStorePath, pageForAssistant } from "./page";

describe("pageForAssistant", () => {
  it("keeps a public page's path and only its search", () => {
    expect(pageForAssistant("/shop/jackets/f/color/v/navy", "?utm_source=mail&sort=x")).toBe("/shop/jackets/f/color/v/navy");
    expect(pageForAssistant("/de/search", "?q=rain+jacket&email=a%40b.c")).toBe("/de/search?q=rain+jacket");
    expect(pageForAssistant("/", "")).toBe("/");
  });

  it.each([
    "/account/reset/abc123",
    "/account/orders/1002",
    "/wishlist/tok-1",
    "/cart/offer",
    "/cart",
    "/checkout",
    "/orders/9f0c",
    "/verify-email",
    "/handoff/tok",
    "/offer/tok",
    "/gift-card",
    "/track-order",
    "/de/account/reset/abc123",
    "/pt-BR/checkout",
    "/0f6c7a52-6f3e-4b55-9d8e-2c1b8f0e9a11/de/wishlist/tok",
    "/0f6c7a52-6f3e-4b55-9d8e-2c1b8f0e9a11/cart",
    "/%61ccount/profile",
    "/Account",
  ])("tells the assistant nothing about %s", (path) => {
    expect(pageForAssistant(path, "?token=secret&q=x")).toBe("");
  });

  it("gives up on a path it can't read", () => {
    expect(pageForAssistant("/%E0%A4%A", "")).toBe("");
    expect(pageForAssistant("//evil.example/x", "")).toBe("");
    expect(pageForAssistant("shop", "")).toBe("");
  });

  it("doesn't mistake a short store segment for a locale", () => {
    expect(pageForAssistant("/cms/preview/7", "?token=t")).toBe("/cms/preview/7");
  });
});

describe("isStorePath", () => {
  it("takes a path on this store and nothing a browser reads as another host", () => {
    expect(isStorePath("/shop/mugs")).toBe(true);
    expect(isStorePath("//evil.example")).toBe(false);
    expect(isStorePath("/\\evil.example")).toBe(false);
    expect(isStorePath("/\t/evil.example")).toBe(false);
    expect(isStorePath("https://evil.example")).toBe(false);
    expect(isStorePath("")).toBe(false);
  });
});
