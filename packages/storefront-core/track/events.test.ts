import { describe, expect, it } from "vitest";

import { cleanUrl, trackPayload } from "./events";

describe("browser tracking", () => {
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
