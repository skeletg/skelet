import { describe, expect, it } from "vitest";

import { clearRewriteMark, isOwnRewrite, markRewrite } from "./rewrite-mark";

describe("rewrite mark", () => {
  it("recognises the mark this process set", () => {
    const headers = new Headers();
    markRewrite(headers);
    expect(isOwnRewrite(headers)).toBe(true);
  });

  it("refuses a request without it, and one a client made up", () => {
    expect(isOwnRewrite(new Headers())).toBe(false);
    expect(isOwnRewrite(new Headers({ "x-storefront-rewrite": "1" }))).toBe(false);
    const forged = new Headers();
    markRewrite(forged);
    const value = forged.get("x-storefront-rewrite") ?? "";
    const flipped = value.slice(0, -1) + (value.endsWith("0") ? "1" : "0");
    expect(isOwnRewrite(new Headers({ "x-storefront-rewrite": flipped }))).toBe(false);
  });

  it("clears it, so the render never sees it", () => {
    const headers = new Headers();
    markRewrite(headers);
    clearRewriteMark(headers);
    expect(headers.has("x-storefront-rewrite")).toBe(false);
    expect(isOwnRewrite(headers)).toBe(false);
  });
});
