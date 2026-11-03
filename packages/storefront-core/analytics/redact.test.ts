import { describe, expect, it } from "vitest";

import { redactPath } from "./redact";

describe("redactPath", () => {
  it("keeps plain paths untouched", () => {
    expect(redactPath("/")).toBe("/");
    expect(redactPath("")).toBe("/");
    expect(redactPath("/account/orders")).toBe("/account/orders");
    expect(redactPath("/blog/how-to-choose-winter-tyres")).toBe(
      "/blog/how-to-choose-winter-tyres",
    );
    expect(redactPath("/collections/summer-sale-2026")).toBe(
      "/collections/summer-sale-2026",
    );
  });

  it("redacts uuid and numeric segments", () => {
    expect(
      redactPath("/products/2b5d9d3e-1f4a-4c2b-9f0e-7a1c3d5e7f90"),
    ).toBe("/products/:id");
    expect(redactPath("/account/orders/1042")).toBe("/account/orders/:id");
  });

  it("redacts a live password-reset token", () => {
    const token = "a".repeat(64);
    expect(redactPath(`/account/reset/${token}`)).toBe("/account/reset/:token");
    expect(
      redactPath(
        "/account/reset/9f2c8b1de4a70536cc91ab4e2f8d07135a6be9c04d21738fa5e6c0b91d47e832",
      ),
    ).toBe("/account/reset/:token");
  });

  it("redacts opaque base64url tokens", () => {
    expect(redactPath("/wishlist/Zm9vYmFyQmF6UXV4MTIzNDU2Nzg5")).toBe(
      "/wishlist/:token",
    );
    expect(redactPath("/download/tok3nWith0utSeparators1")).toBe(
      "/download/:token",
    );
  });

  it("does not over-redact long legitimate slugs", () => {
    expect(redactPath("/michelin-pilot-sport-5-225-45-r17")).toBe(
      "/michelin-pilot-sport-5-225-45-r17",
    );
    expect(redactPath("/pages/terms_and_conditions_full")).toBe(
      "/pages/terms_and_conditions_full",
    );
    expect(redactPath("/categories/uncategorizedproducts")).toBe(
      "/categories/uncategorizedproducts",
    );
  });

  it("drops any query or fragment that rides along", () => {
    expect(redactPath("/search?q=secret#top")).toBe("/search");
  });
});
