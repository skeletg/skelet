import { describe, expect, it } from "vitest";

import { isPrivateRecordingPath } from "@/app/lib/analytics/recording-policy";

describe("isPrivateRecordingPath", () => {
  it("excludes shopper-private pages, with or without a locale prefix", () => {
    for (const path of [
      "/account",
      "/account/orders/123",
      "/account/reset/abc",
      "/cart",
      "/cart/offer",
      "/checkout",
      "/orders/5b0e7c1e-0000-4000-8000-000000000000",
      "/track-order",
      "/gift-card",
      "/wishlist/sharedtoken123",
      "/cms/preview/7",
      "/blog/preview/7",
    ]) {
      expect(isPrivateRecordingPath(path), path).toBe(true);
      expect(isPrivateRecordingPath("/de" + path), "/de" + path).toBe(true);
      expect(isPrivateRecordingPath("/pt-br" + path), "/pt-br" + path).toBe(true);
    }
    expect(isPrivateRecordingPath("/checkout?step=payment#top")).toBe(true);
  });

  it("keeps public pages recordable", () => {
    for (const path of [
      "/",
      "",
      "/de",
      "/products/blue-shirt",
      "/de/products/account",
      "/search?q=cart",
      "/wishlist",
      "/de/wishlist",
      "/blog/checkout-tips",
      "/categories/gift-cards",
    ]) {
      expect(isPrivateRecordingPath(path), path).toBe(false);
    }
  });
});
