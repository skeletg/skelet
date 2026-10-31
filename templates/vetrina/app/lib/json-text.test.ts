import { describe, expect, it } from "vitest";

import { renderJsonText } from "@/app/lib/json-text";

describe("renderJsonText", () => {
  it("returns null for empty input", () => {
    expect(renderJsonText(null)).toBeNull();
    expect(renderJsonText(undefined)).toBeNull();
    expect(renderJsonText("   ")).toBeNull();
  });

  it("trims plain strings and stringifies scalars", () => {
    expect(renderJsonText("  hello ")).toBe("hello");
    expect(renderJsonText(42)).toBe("42");
    expect(renderJsonText(true)).toBe("true");
  });

  it("prefers the plain copy of a { text, plain } wrapper without doubling it", () => {
    expect(renderJsonText({ text: "Same", plain: "Same" })).toBe("Same");
    expect(renderJsonText({ text: "Only text" })).toBe("Only text");
  });

  it("walks a rich-text tree and joins its string leaves", () => {
    const doc = {
      root: { children: [{ children: [{ text: "First" }] }, { children: [{ text: "Second" }] }] },
    };
    expect(renderJsonText(doc)).toBe("First Second");
  });

  it("drops adjacent duplicate leaves", () => {
    expect(renderJsonText(["a", "a", "b"])).toBe("a b");
  });
});
