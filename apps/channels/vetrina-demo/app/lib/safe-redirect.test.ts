import { describe, expect, it } from "vitest";

import { safeRelativePath } from "@/app/lib/safe-redirect";

describe("safeRelativePath", () => {
  it("passes clean relative paths through", () => {
    expect(safeRelativePath("/products/tire?page=2#reviews")).toBe(
      "/products/tire?page=2#reviews",
    );
  });

  it.each([
    ["absolute URL", "https://evil.com/phish"],
    ["protocol-relative", "//evil.com"],
    ["backslash smuggle", "/\\evil.com"],
    ["backslash anywhere", "/ok\\..\\evil"],
    ["missing leading slash", "products"],
    ["javascript scheme", "javascript:alert(1)"],
    ["empty", ""],
  ])("rejects %s", (_label, input) => {
    expect(safeRelativePath(input)).toBe("/");
  });

  it("rejects null", () => {
    expect(safeRelativePath(null)).toBe("/");
  });
});
