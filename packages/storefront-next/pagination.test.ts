import { describe, expect, it } from "vitest";

import {
  CUMULATIVE_PAGE_CAP,
  hrefForPage,
  mergeSlices,
  parsePage,
  planSlices,
} from "./pagination";

describe("parsePage", () => {
  it("defaults to 1 for absent, garbage, zero and negative values", () => {
    expect(parsePage({})).toBe(1);
    expect(parsePage({ page: "banana" })).toBe(1);
    expect(parsePage({ page: "0" })).toBe(1);
    expect(parsePage({ page: "-3" })).toBe(1);
  });

  it("takes the first value of a repeated param", () => {
    expect(parsePage({ page: ["4", "9"] })).toBe(4);
  });
});

describe("hrefForPage", () => {
  // a /pg/1 segment would be a crawlable duplicate of the clean URL
  it("omits the page segment for page 1", () => {
    expect(hrefForPage("/shop/apparel", {}, 1)).toBe("/shop/apparel");
  });

  // the page joins the facet/sort chain in the path rather than becoming a query param
  it("hangs /pg/<n> off the end of the chain", () => {
    expect(hrefForPage("/shop/apparel/f/color/v/red/s/price-asc", {}, 4)).toBe(
      "/shop/apparel/f/color/v/red/s/price-asc/pg/4",
    );
  });

  // q is a search term, not part of the listing path grammar - it stays in the query
  it("keeps preserved params in the query string", () => {
    expect(hrefForPage("/search", { q: "tee" }, 3)).toBe("/search/pg/3?q=tee");
  });
});

describe("planSlices", () => {
  it("plans one slice per page up to the requested page", () => {
    expect(planSlices(1, 24)).toEqual({ cumulative: true, offsets: [0] });
    expect(planSlices(5, 24)).toEqual({ cumulative: true, offsets: [0, 24, 48, 72, 96] });
  });

  // the cap is what keeps a crawler's ?page=400 from fanning out 400 backend queries
  it("degrades to a single windowed query past the cap", () => {
    const atCap = planSlices(CUMULATIVE_PAGE_CAP, 24);
    expect(atCap.cumulative).toBe(true);
    const pastCap = planSlices(CUMULATIVE_PAGE_CAP + 1, 24);
    expect(pastCap).toEqual({ cumulative: false, offset: CUMULATIVE_PAGE_CAP * 24 });
  });
});

describe("mergeSlices", () => {
  const p = (id: string) => ({ id });

  // an item shifting across a slice boundary between queries must not render twice -
  // duplicate ids would also collide as React keys
  it("drops duplicates across slice boundaries, first occurrence wins", () => {
    expect(mergeSlices([[p("a"), p("b")], [p("b"), p("c")]]).map((x) => x.id)).toEqual([
      "a",
      "b",
      "c",
    ]);
  });

  it("tolerates short and empty slices", () => {
    expect(mergeSlices([[p("a")], [], [p("b")]]).map((x) => x.id)).toEqual(["a", "b"]);
  });
});
