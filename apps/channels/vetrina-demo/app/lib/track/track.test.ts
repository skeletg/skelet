import { describe, expect, it } from "vitest";

import { cleanUrl, isBrowserEvent, trackPayload } from "@/app/lib/track";

describe("browser tracking", () => {
  it("sends only the events a browser may report", () => {
    expect(isBrowserEvent("view_item")).toBe(true);
    for (const name of ["purchase", "add_to_cart", "sign_up", "refund"]) expect(isBrowserEvent(name)).toBe(false);
  });

  it("drops a query string that could carry a guest's email, keeping campaign tags", () => {
    expect(cleanUrl("https://shop.example/orders/1?email=a%40b.c&utm_source=news#x")).toBe("https://shop.example/orders/1?utm_source=news");
    expect(cleanUrl("not a url")).toBe("");
  });

  it("shapes the payload the backend reads", () => {
    expect(trackPayload("view_item", "e-1", { currency: "EUR" }, "https://shop.example/p?token=t", "", "en")).toEqual({
      event_id: "e-1",
      name: "view_item",
      ecommerce: { currency: "EUR" },
      page_url: "https://shop.example/p",
      referrer: "",
      locale: "en",
    });
  });
});
