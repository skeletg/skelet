import { describe, expect, it } from "vitest";

import { lexicalHasContent, lexicalSummary } from "./lexical";

describe("lexicalSummary", () => {
  it("returns empty for no content", () => {
    expect(lexicalSummary(null)).toBe("");
    expect(lexicalSummary("   ")).toBe("");
  });

  it("returns short text whole, whitespace collapsed", () => {
    expect(lexicalSummary("Soft  cotton\n tee.")).toBe("Soft cotton tee.");
  });

  it("cuts long text on a word boundary within the limit", () => {
    const out = lexicalSummary("alpha beta gamma delta epsilon, zeta", 30);
    expect(out).toBe("alpha beta gamma delta…");
    expect(out.length).toBeLessThanOrEqual(30);
  });

  it("hard-cuts a single overlong word", () => {
    const out = lexicalSummary("x".repeat(50), 20);
    expect(out).toBe(`${"x".repeat(19)}…`);
  });
});

describe("lexicalHasContent", () => {
  const doc = (...children: unknown[]) => ({ root: { type: "root", children } });

  it("counts text and custom blocks", () => {
    expect(lexicalHasContent("Soft cotton")).toBe(true);
    expect(lexicalHasContent(doc({ type: "paragraph", children: [{ type: "text", text: "Soft cotton" }] }))).toBe(true);
    expect(lexicalHasContent(doc({ type: "faq-block", items: [], version: 1 }))).toBe(true);
    expect(lexicalHasContent(doc({ type: "paragraph", children: [{ type: "product-image", src: "/a.png", altText: "", width: 1, height: 1, version: 1 }] }))).toBe(true);
  });

  it("calls an editor someone emptied empty", () => {
    expect(lexicalHasContent(doc({ type: "paragraph", children: [] }))).toBe(false);
    expect(lexicalHasContent(doc({ type: "paragraph", children: [{ type: "text", text: "  " }, { type: "linebreak" }] }, { type: "heading", tag: "h2", children: [] }))).toBe(false);
    expect(lexicalHasContent(null)).toBe(false);
    expect(lexicalHasContent("  ")).toBe(false);
  });
});
