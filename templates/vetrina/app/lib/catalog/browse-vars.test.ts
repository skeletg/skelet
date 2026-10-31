import { describe, expect, it } from "vitest";

import { normalizeBrowseVars } from "@/app/lib/catalog/browse-vars";

describe("normalizeBrowseVars", () => {
  it("treats a null and an absent query, sort or filter field as the same request", () => {
    const seo = normalizeBrowseVars({ filter: { categoryId: "c1", attributes: null }, limit: 24, offset: 0 });
    const grid = normalizeBrowseVars({
      query: null,
      filter: { categoryId: "c1", attributes: null },
      sort: null,
      limit: 24,
      offset: 0,
    });
    expect(grid).toEqual(seo);
    expect(seo).toEqual({ filter: { categoryId: "c1" }, limit: 24, offset: 0 });
  });

  it("drops a filter left empty", () => {
    expect(normalizeBrowseVars({ filter: { attributes: null }, limit: 24 })).toEqual({ limit: 24 });
  });

  it("keeps an explicit null limit or offset, since those carry defaults", () => {
    expect(normalizeBrowseVars({ limit: null, offset: null })).toEqual({ limit: null, offset: null });
  });

  it("leaves real values alone", () => {
    const attrs = [{ slug: "color", values: ["red"] }];
    expect(normalizeBrowseVars({ query: "tyre", sort: "price_asc", filter: { attributes: attrs } })).toEqual({
      query: "tyre",
      sort: "price_asc",
      filter: { attributes: attrs },
    });
  });
});
