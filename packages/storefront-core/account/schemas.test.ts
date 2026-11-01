import { describe, expect, it } from "vitest";

import { returnRequestSchema } from "./schemas";
import { parseForm } from "../forms/schema";

function form(entries: [string, string][]): FormData {
  const fd = new FormData();
  for (const [k, v] of entries) fd.append(k, v);
  return fd;
}

describe("returnRequestSchema", () => {
  it("reads the indexed lines as numbers, blank as 0", () => {
    const s = parseForm(
      form([
        ["orderId", "o1"],
        ["lines[0].orderLineId", "l1"],
        ["lines[0].quantity", "2"],
        ["lines[1].orderLineId", "l2"],
        ["lines[1].quantity", ""],
      ]),
      returnRequestSchema,
    );
    expect(s.status === "success" && s.value).toEqual({
      orderId: "o1",
      lines: [
        { orderLineId: "l1", quantity: 2 },
        { orderLineId: "l2", quantity: 0 },
      ],
      resolution: "refund",
      reason: null,
    });
  });

  it("flags a fractional quantity on its line and an all-zero return on the form", () => {
    const frac = parseForm(
      form([
        ["orderId", "o1"],
        ["lines[0].orderLineId", "l1"],
        ["lines[0].quantity", "1.5"],
      ]),
      returnRequestSchema,
    );
    expect(frac.status === "error" && frac.error?.["lines[0].quantity"]).toEqual([
      "wholeNumber",
    ]);

    const none = parseForm(
      form([
        ["orderId", "o1"],
        ["lines[0].orderLineId", "l1"],
        ["lines[0].quantity", "0"],
      ]),
      returnRequestSchema,
    );
    expect(none.status === "error" && none.error?.[""]).toEqual([
      "returnNothingSelected",
    ]);
  });
});
