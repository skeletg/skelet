import { describe, expect, it } from "vitest";

import { stepLine } from "./steps";

describe("stepLine", () => {
  it("names what the step is about when there's something to name", () => {
    expect(stepLine({ tool: "search_products", detail: "rain jacket" }, "en")).toEqual({ key: "steps.searchFor", detail: "rain jacket" });
    expect(stepLine({ tool: "search_products", detail: "" }, "en")).toEqual({ key: "steps.search", detail: "" });
    expect(stepLine({ tool: "get_cart", detail: "ignored" }, "en")).toEqual({ key: "steps.cart", detail: "ignored" });
  });

  it("names the order the shopper asked about", () => {
    expect(stepLine({ tool: "get_my_order", detail: "1002" }, "en")).toEqual({ key: "steps.orderNamed", detail: "1002" });
    expect(stepLine({ tool: "start_return", detail: "1002" }, "en")).toEqual({ key: "steps.returnFor", detail: "1002" });
    expect(stepLine({ tool: "get_my_orders", detail: "" }, "en")).toEqual({ key: "steps.orders", detail: "" });
  });

  it("names a delivery country in the shopper's language", () => {
    expect(stepLine({ tool: "estimate_shipping", detail: "DE" }, "en")).toEqual({ key: "steps.deliveryTo", detail: "Germany" });
    expect(stepLine({ tool: "estimate_shipping", detail: "DE" }, "de")).toEqual({ key: "steps.deliveryTo", detail: "Deutschland" });
  });

  it("leaves a tool it doesn't know to the plain thinking line", () => {
    expect(stepLine({ tool: "something_new", detail: "x" }, "en")).toBeNull();
    for (const tool of ["constructor", "toString", "__proto__", "hasOwnProperty"]) expect(stepLine({ tool, detail: "x" }, "en")).toBeNull();
    expect(stepLine(null, "en")).toBeNull();
  });
});
