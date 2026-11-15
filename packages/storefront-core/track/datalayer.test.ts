import { afterEach, describe, expect, it } from "vitest";

import { eventAttrs, pushEvent } from "./datalayer";

describe("the dataLayer", () => {
  afterEach(() => {
    delete (globalThis as { window?: unknown }).window;
  });

  it("pushes an event in GA4's shape, clearing the last one's ecommerce first", () => {
    (globalThis as { window?: unknown }).window = {};
    const ecommerce = { currency: "EUR", value: 12.9, items: [{ item_id: "SKU-1" }] };
    const id = pushEvent("view_item", { ecommerce }, "e-1");
    pushEvent("page_view", {}, "e-2");
    expect(id).toBe("e-1");
    expect(window.dataLayer).toEqual([
      { ecommerce: null },
      { event: "view_item", event_id: "e-1", ecommerce },
      { event: "page_view", event_id: "e-2" },
    ]);
  });

  it("does nothing on the server", () => {
    expect(pushEvent("page_view", {}, "e-3")).toBe("e-3");
  });

  it("carries the same event as markup attributes", () => {
    expect(eventAttrs("view_item", { ecommerce: { currency: "EUR", items: [{ item_id: "SKU-1" }] } })).toEqual({
      "data-track": "view_item",
      "data-track-data": '{"ecommerce":{"currency":"EUR","items":[{"item_id":"SKU-1"}]}}',
    });
  });
});
