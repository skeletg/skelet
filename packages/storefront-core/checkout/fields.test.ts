import { describe, expect, it } from "vitest";

import { fieldAnswer, fieldIssue, fieldsComplete, type CheckoutFieldSpec } from "./fields";

const field = (f: Partial<CheckoutFieldSpec>): CheckoutFieldSpec => ({
  key: "f", label: "F", hint: "", type: "TEXT", options: [], required: false, maxLength: 5, value: "", ...f,
});

describe("checkout fields", () => {
  it("reads a ticked box as true and an unticked one as no answer", () => {
    const tick = field({ type: "CHECKBOX" });
    expect(fieldAnswer(tick, "on")).toBe("true");
    expect(fieldAnswer(tick, undefined)).toBe("");
    expect(fieldAnswer(field({}), "  hi  ")).toBe("hi");
  });

  it.each([
    [field({ required: true }), "", "required"],
    [field({ type: "CHECKBOX", required: true }), undefined, "fieldTick"],
    [field({}), "toolong", "checkoutFieldTooLong"],
    [field({}), "", null],
    [field({ type: "SELECT", options: [{ value: "paper", label: "Paper" }, { value: "box", label: "Box" }] }), "bag", "fieldInvalid"],
    [field({ type: "SELECT", options: [{ value: "paper", label: "Paper" }, { value: "box", label: "Box" }] }), "box", null],
    [field({ type: "DATE" }), "2026-02-31", "fieldInvalid"],
    [field({ type: "DATE" }), "2026-12-24", null],
  ] as const)("judges %o's answer %o", (f, raw, why) => {
    expect(fieldIssue(f, raw)).toBe(why);
  });

  it("keeps the step open while a required field has no answer", () => {
    expect(fieldsComplete([field({ required: true })])).toBe(false);
    expect(fieldsComplete([field({ required: true, value: "x" }), field({})])).toBe(true);
  });
});
