import { describe, expect, it } from "vitest";

import { renderJsonText } from "./json-text";

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

  it("reads a Lexical document as its text, leaving out the node bookkeeping", () => {
    const text = (t: string) => ({ type: "text", text: t, format: 0, mode: "normal", style: "" });
    const doc = {
      root: {
        type: "root",
        direction: "ltr",
        children: [
          { type: "paragraph", direction: "ltr", children: [text("First")] },
          { type: "heading", tag: "h4", children: [text("Care")] },
          { type: "list", listType: "bullet", tag: "ul", children: [{ type: "listitem", children: [text("Wash cold")] }] },
        ],
      },
    };
    expect(renderJsonText(doc)).toBe("First Care Wash cold");
  });

  it("walks any other tree and joins its string leaves", () => {
    expect(renderJsonText({ body: [{ caption: "First" }, { caption: "Second" }] })).toBe("First Second");
  });

  it("drops adjacent duplicate leaves", () => {
    expect(renderJsonText(["a", "a", "b"])).toBe("a b");
  });
});
