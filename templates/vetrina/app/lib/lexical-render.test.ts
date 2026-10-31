import { describe, expect, it } from "vitest";

import { lexicalSummary } from "@/app/lib/lexical-render";

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
