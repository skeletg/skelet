import { describe, expect, it } from "vitest";

import { assistantHref } from "./AssistantLinks";

describe("assistantHref", () => {
  it("writes a search the way the search page reads it", () => {
    expect(
      assistantHref({ label: "x", path: "/search", query: "rain jacket", filters: [{ slug: "size", values: ["m"] }, { slug: "color", values: ["red", "navy"] }], sort: "price_asc", count: 3 }),
    ).toBe("/search/f/color/v/navy/v/red/f/size/v/m/s/price_asc?q=rain+jacket");
  });

  it("narrows a category in its own path, and leaves a page as it is", () => {
    expect(assistantHref({ label: "x", path: "/shop/jackets", query: "", filters: [{ slug: "size", values: ["m"] }], sort: "", count: 9 })).toBe("/shop/jackets/f/size/v/m");
    expect(assistantHref({ label: "x", path: "/info/returns", query: "", filters: [], sort: "", count: null })).toBe("/info/returns");
  });
});
