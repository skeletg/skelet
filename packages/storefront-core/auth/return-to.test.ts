import { describe, expect, it } from "vitest";

import { safeReturnTo } from "./return-to";

describe("safeReturnTo", () => {
  it("passes a path on this site through", () => {
    expect(safeReturnTo("/products/tire?page=2#reviews")).toBe("/products/tire?page=2#reviews");
    expect(safeReturnTo(["/account", "/other"])).toBe("/account");
    expect(safeReturnTo("/%2F%2Fevil.com")).toBe("/%2F%2Fevil.com");
  });

  it.each([
    ["an absolute URL", "https://evil.com/phish"],
    ["a protocol-relative one", "//evil.com"],
    ["a backslash a browser folds to a slash", "/\\evil.com"],
    ["a backslash anywhere", "/ok\\..\\evil"],
    ["a tab a browser drops", "/\t/evil.com"],
    ["a newline a browser drops", "/\n/evil.com"],
    ["a carriage return", "/\r/evil.com"],
    ["a NUL", "/\u0000/evil.com"],
    ["a DEL", "/\u007f/evil.com"],
    ["a space", "/ /evil.com"],
    ["no leading slash", "products"],
    ["a javascript: URL", "javascript:alert(1)"],
    ["nothing", ""],
    ["not a string", 42],
    ["null", null],
  ])("refuses %s", (_label, input) => {
    expect(safeReturnTo(input)).toBeNull();
  });
});
