import { readdirSync, readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

import { BROWSER_EVENT_NAMES, type BrowserEventName } from "./contract.gen";
import { cleanUrl, trackPayload } from "./events";

describe("browser tracking", () => {
  it("drops a query string that could carry a guest's email, keeping campaign tags", () => {
    expect(cleanUrl("https://shop.example/orders/1?email=a%40b.c&utm_source=news#x")).toBe("https://shop.example/orders/1?utm_source=news");
    expect(cleanUrl("not a url")).toBe("");
  });

  it("shapes the payload the backend reads", () => {
    expect(trackPayload("view_item", "e-1", { ecommerce: { currency: "EUR" } }, { href: "https://shop.example/p?token=t", referrer: "", locale: "en" })).toEqual({
      event_id: "e-1",
      name: "view_item",
      ecommerce: { currency: "EUR" },
      page_url: "https://shop.example/p",
      referrer: "",
      locale: "en",
    });
  });
});

describe("GA4 items and search", () => {
  it("names a product by its SKU at the price shown, with its place in the list", async () => {
    const { ga4Item } = await import("./items");
    const p = { sku: "SKU-1", name: "Mug", price: 1290, defaultPrice: 1500, defaultCurrencyCode: "EUR" };
    expect(ga4Item(p, 0, { id: "/c/mugs", name: "Mugs" })).toEqual({
      item_id: "SKU-1",
      item_name: "Mug",
      price: 12.9,
      index: 0,
      item_list_id: "/c/mugs",
      item_list_name: "Mugs",
    });
  });

  it("shapes a cart's lines as the cart's value and items", async () => {
    const { ga4CartEcommerce } = await import("./items");
    const line = { id: "l1", name: "Mug", sku: "SKU-1", unitPrice: 1290, quantity: 2, totalPrice: 2580, components: [], options: [] };
    expect(ga4CartEcommerce([line], 2580, { code: "EUR", symbol: "€" })).toEqual({
      currency: "EUR",
      value: 25.8,
      items: [{ item_id: "SKU-1", item_name: "Mug", quantity: 2, price: 12.9 }],
    });
  });

  it("carries a search term apart from the url, trimmed to what the backend takes", () => {
    const body = trackPayload("search", "e-2", { searchTerm: ` ${"x".repeat(300)} ` }, { href: "https://shop.example/search?q=mug", referrer: "", locale: "en" });
    expect(body.page_url).toBe("https://shop.example/search");
    expect(body.search_term).toHaveLength(200);
  });
});

// The bodies core's Go tests read (apps/backend/sdk/testdata/events), one per browser event: what trackPayload builds
// from an event's parts must be exactly the body core was shown to accept whole
describe("the golden browser events", () => {
  const dir = new URL("../../../apps/backend/sdk/testdata/events/", import.meta.url);
  const files = readdirSync(dir).filter((f) => f.endsWith(".json"));

  it("has a body for every event a browser may send", () => {
    expect(files.map((f) => f.replace(".json", "")).sort()).toEqual([...BROWSER_EVENT_NAMES].sort());
  });

  it.each(files)("%s is what trackPayload sends", (file) => {
    const golden = JSON.parse(readFileSync(new URL(file, dir), "utf8"));
    const body = trackPayload(golden.name as BrowserEventName, golden.event_id, { ecommerce: golden.ecommerce, searchTerm: golden.search_term }, {
      href: golden.page_url,
      referrer: golden.referrer,
      locale: golden.locale,
      title: golden.page_title,
    });
    expect(JSON.parse(JSON.stringify(body))).toEqual(golden);
  });
});
