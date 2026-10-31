import { describe, expect, it } from "vitest";

import {
  assembleH1,
  buildFilterPath,
  evaluateFilterIndexability,
  isChainStart,
  isPagedUrlIndexable,
  isSortUrlIndexable,
  joinBasePathWithFilters,
  parseFilterSegments,
  parsePageFromSegments,
  parseSortFromSegments,
  splitSlug,
  withPageSegment,
  toggleFilterValue,
  type FacetGroupForIndexability,
} from "@/app/lib/filter-path";

// These URLs are what Google indexes, so the two rules that matter are: reading is tolerant (a
// hand-edited path renders unfiltered rather than 500ing) and writing is canonical (two click
// paths to the same filter state must produce one url, or the same page competes with itself)

const bucket = (slug: string, over: Partial<{ value: string; isIndexable: boolean; description: string }> = {}) => ({
  slug,
  value: slug,
  isIndexable: true,
  description: "",
  ...over,
});

const facet = (slug: string, buckets: ReturnType<typeof bucket>[], h1Position: "BEFORE" | "AFTER" = "BEFORE"): FacetGroupForIndexability =>
  ({ slug, h1Position, buckets });

describe("splitSlug", () => {
  it("treats a path with no chain as all entity", () => {
    expect(splitSlug(["shoes", "boots"])).toEqual({
      entityPath: "shoes/boots",
      filters: [],
      sortSlug: "",
      page: 1,
    });
  });

  it("splits the entity from its filter chain", () => {
    expect(splitSlug(["shoes", "f", "brand", "v", "atelier"])).toEqual({
      entityPath: "shoes",
      filters: [{ slug: "brand", values: ["atelier"] }],
      sortSlug: "",
      page: 1,
    });
  });

  it("picks up a sort at the end of the chain", () => {
    const out = splitSlug(["shoes", "f", "brand", "v", "atelier", "s", "price-asc"]);
    expect(out.sortSlug).toBe("price-asc");
    expect(out.filters).toEqual([{ slug: "brand", values: ["atelier"] }]);
  });

  it("handles a chain with only a sort", () => {
    expect(splitSlug(["shoes", "s", "newest"])).toEqual({
      entityPath: "shoes",
      filters: [],
      sortSlug: "newest",
      page: 1,
    });
  });

  it("picks up a page at the end of the chain", () => {
    const out = splitSlug(["shoes", "f", "brand", "v", "atelier", "s", "price-asc", "pg", "2"]);
    expect(out.page).toBe(2);
    expect(out.sortSlug).toBe("price-asc");
    expect(out.filters).toEqual([{ slug: "brand", values: ["atelier"] }]);
  });

  // /shop/pg/2 - a page segment is a chain start in its own right, no filters needed
  it("splits a page-only chain off the entity", () => {
    expect(splitSlug(["shoes", "pg", "3"])).toEqual({
      entityPath: "shoes",
      filters: [],
      sortSlug: "",
      page: 3,
    });
  });

  it("percent-decodes each segment", () => {
    expect(splitSlug(["f", "size", "v", "42%C2%BD"]).filters).toEqual([
      { slug: "size", values: ["42½"] },
    ]);
  });

  it("knows which segments open a chain", () => {
    expect(isChainStart("f")).toBe(true);
    expect(isChainStart("s")).toBe(true);
    expect(isChainStart("v")).toBe(false);
    expect(isChainStart("shoes")).toBe(false);
  });
});

describe("parseFilterSegments", () => {
  it("gathers every value under its attribute", () => {
    expect(parseFilterSegments(["f", "brand", "v", "atelier", "v", "cream"])).toEqual([
      { slug: "brand", values: ["atelier", "cream"] },
    ]);
  });

  it("keeps several attributes apart", () => {
    expect(parseFilterSegments(["f", "brand", "v", "a", "f", "size", "v", "42"])).toEqual([
      { slug: "brand", values: ["a"] },
      { slug: "size", values: ["42"] },
    ]);
  });

  // an attribute with no values re-emits as /f/brand, which round-trips but means "no filter"
  it("drops an attribute nobody picked a value for", () => {
    expect(parseFilterSegments(["f", "brand", "f", "size", "v", "42"])).toEqual([
      { slug: "size", values: ["42"] },
    ]);
  });

  it("skips a value with no attribute in front of it", () => {
    expect(parseFilterSegments(["v", "orphan", "f", "brand", "v", "a"])).toEqual([
      { slug: "brand", values: ["a"] },
    ]);
  });

  it("stops cleanly on a trailing marker rather than reading past the end", () => {
    expect(parseFilterSegments(["f", "brand", "v", "a", "f"])).toEqual([
      { slug: "brand", values: ["a"] },
    ]);
    expect(parseFilterSegments(["f", "brand", "v"])).toEqual([]);
  });

  it("ignores markers it does not know instead of filtering on them", () => {
    expect(parseFilterSegments(["f", "brand", "v", "a", "x", "junk"])).toEqual([
      { slug: "brand", values: ["a"] },
    ]);
  });

  it("does not mistake a sort payload for an attribute", () => {
    expect(parseFilterSegments(["s", "price-asc", "f", "brand", "v", "a"])).toEqual([
      { slug: "brand", values: ["a"] },
    ]);
  });

  it("finds nothing in an empty chain", () => {
    expect(parseFilterSegments([])).toEqual([]);
  });
});

describe("parseSortFromSegments", () => {
  it("reads the sort slug", () => {
    expect(parseSortFromSegments(["s", "price-asc"])).toBe("price-asc");
  });

  // hand-edited duplicates collapse, same tolerant-on-read rule the filters follow
  it("takes the first of several", () => {
    expect(parseSortFromSegments(["s", "price-asc", "s", "newest"])).toBe("price-asc");
  });

  it("answers empty for a dangling or absent marker", () => {
    expect(parseSortFromSegments(["s"])).toBe("");
    expect(parseSortFromSegments(["f", "brand", "v", "a"])).toBe("");
  });
});

describe("buildFilterPath", () => {
  it("emits the chain grammar", () => {
    expect(buildFilterPath([{ slug: "brand", values: ["atelier"] }])).toBe("f/brand/v/atelier");
  });

  // the whole point: clicking brand-then-size and size-then-brand must give ONE url to index
  it("sorts attributes and values, so click order cannot change the url", () => {
    const one = buildFilterPath([
      { slug: "size", values: ["44", "42"] },
      { slug: "brand", values: ["cream", "atelier"] },
    ]);
    const other = buildFilterPath([
      { slug: "brand", values: ["atelier", "cream"] },
      { slug: "size", values: ["42", "44"] },
    ]);
    expect(one).toBe(other);
    expect(one).toBe("f/brand/v/atelier/v/cream/f/size/v/42/v/44");
  });

  it("leaves out an attribute with no values", () => {
    expect(buildFilterPath([{ slug: "brand", values: [] }])).toBe("");
  });

  it("percent-encodes so a slash in a value cannot forge a segment", () => {
    expect(buildFilterPath([{ slug: "brand", values: ["a/b"] }])).toBe("f/brand/v/a%2Fb");
  });

  it("does not reorder the caller’s own array", () => {
    const input = [{ slug: "size", values: ["44", "42"] }, { slug: "brand", values: ["b"] }];
    buildFilterPath(input);
    expect(input[0].slug).toBe("size");
    expect(input[0].values).toEqual(["44", "42"]);
  });

  it("round-trips through the parser", () => {
    const filters = [
      { slug: "brand", values: ["atelier", "cream"] },
      { slug: "size", values: ["42"] },
    ];
    expect(parseFilterSegments(buildFilterPath(filters).split("/"))).toEqual(filters);
  });
});

describe("toggleFilterValue", () => {
  it("adds an attribute that was not there", () => {
    expect(toggleFilterValue([], "brand", "atelier")).toEqual([{ slug: "brand", values: ["atelier"] }]);
  });

  it("adds another value to an attribute already in play", () => {
    expect(toggleFilterValue([{ slug: "brand", values: ["a"] }], "brand", "b")).toEqual([
      { slug: "brand", values: ["a", "b"] },
    ]);
  });

  it("removes a value that was already picked", () => {
    expect(toggleFilterValue([{ slug: "brand", values: ["a", "b"] }], "brand", "a")).toEqual([
      { slug: "brand", values: ["b"] },
    ]);
  });

  it("drops the attribute once its last value goes", () => {
    expect(toggleFilterValue([{ slug: "brand", values: ["a"] }], "brand", "a")).toEqual([]);
  });

  it("leaves the caller’s array alone", () => {
    const input = [{ slug: "brand", values: ["a"] }];
    toggleFilterValue(input, "brand", "b");
    expect(input).toEqual([{ slug: "brand", values: ["a"] }]);
  });
});

describe("parsePageFromSegments", () => {
  it("reads /pg/<n> out of the chain", () => {
    expect(parsePageFromSegments(["f", "color", "v", "red", "s", "price-asc", "pg", "3"])).toBe(3);
  });

  // tolerant-on-read, like the filter and sort parsers - a typo renders page 1, never a 404
  it("falls back to page 1 for a missing, zero, or unparseable segment", () => {
    expect(parsePageFromSegments(["f", "color", "v", "red"])).toBe(1);
    expect(parsePageFromSegments(["pg"])).toBe(1);
    expect(parsePageFromSegments(["pg", "banana"])).toBe(1);
    expect(parsePageFromSegments(["pg", "0"])).toBe(1);
  });

  // the page payload must not read as an attribute slug
  it("does not leak the page into the filter chain", () => {
    expect(parseFilterSegments(["pg", "2", "f", "color", "v", "red"])).toEqual([
      { slug: "color", values: ["red"] },
    ]);
  });
});

describe("withPageSegment", () => {
  it("leaves page 1 as the bare path, so there is one URL for the first page", () => {
    expect(withPageSegment("/shop/apparel", 1)).toBe("/shop/apparel");
  });

  it("appends the page last, after the sort", () => {
    expect(withPageSegment("/shop/apparel/s/price-asc", 2)).toBe("/shop/apparel/s/price-asc/pg/2");
  });
});

describe("joinBasePathWithFilters", () => {
  it("returns the base path untouched when nothing is filtered", () => {
    expect(joinBasePathWithFilters("/shoes", [])).toBe("/shoes");
  });

  it("puts filters first and the sort last", () => {
    expect(joinBasePathWithFilters("/shoes", [{ slug: "brand", values: ["a"] }], "price-asc")).toBe(
      "/shoes/f/brand/v/a/s/price-asc",
    );
  });

  it("appends a sort with no filters", () => {
    expect(joinBasePathWithFilters("/shoes", [], "newest")).toBe("/shoes/s/newest");
  });

  it("does not double the slash on a base path that ends in one", () => {
    expect(joinBasePathWithFilters("/shoes/", [{ slug: "brand", values: ["a"] }])).toBe("/shoes/f/brand/v/a");
  });

  it("keeps the root as a slash rather than emptying it", () => {
    expect(joinBasePathWithFilters("/", [])).toBe("/");
  });

  it("carries the query string on the end", () => {
    expect(joinBasePathWithFilters("/shoes", [], "", "page=2")).toBe("/shoes?page=2");
  });
});

describe("isSortUrlIndexable", () => {
  // the default is deliberately flagged NOT indexable, so the "explicit default is still canonical"
  // rule is the only thing that can keep /s/relevance out of noindex
  const options = [{ slug: "relevance", isIndexable: false }, { slug: "price-asc", isIndexable: false }, { slug: "newest", isIndexable: true }];

  it("indexes a page with no sort segment", () => {
    expect(isSortUrlIndexable("", options)).toBe(true);
  });

  // spelling out the default is still the canonical page, so it must not go noindex
  it("indexes the default sort even when spelled out", () => {
    expect(isSortUrlIndexable("relevance", options)).toBe(true);
  });

  it("follows the flag for a non-default sort", () => {
    expect(isSortUrlIndexable("newest", options)).toBe(true);
    expect(isSortUrlIndexable("price-asc", options)).toBe(false);
  });

  it("noindexes a sort nobody offers", () => {
    expect(isSortUrlIndexable("made-up", options)).toBe(false);
  });

  // a soft-failed backend must not take the page's indexability with it
  it("indexes when the option list came back empty", () => {
    expect(isSortUrlIndexable("anything", [])).toBe(true);
  });
});

describe("isPagedUrlIndexable", () => {
  const caps = (filtered: number, unfiltered: number) => ({ filtered, unfiltered });

  // the default: a filtered listing stops at page 1, so facet values can't multiply by page
  // count into an unbounded URL grid, while the plain pager stays open
  it("caps the filtered axis while leaving an uncapped plain pager alone", () => {
    expect(isPagedUrlIndexable(1, true, caps(1, 0))).toBe(true);
    expect(isPagedUrlIndexable(2, true, caps(1, 0))).toBe(false);
    expect(isPagedUrlIndexable(50, false, caps(1, 0))).toBe(true);
  });

  it("indexes filtered pages up to the chosen depth, then stops", () => {
    expect(isPagedUrlIndexable(3, true, caps(3, 0))).toBe(true);
    expect(isPagedUrlIndexable(4, true, caps(3, 0))).toBe(false);
  });

  it("caps the unfiltered axis on its own number", () => {
    expect(isPagedUrlIndexable(5, false, caps(1, 5))).toBe(true);
    expect(isPagedUrlIndexable(6, false, caps(1, 5))).toBe(false);
  });

  // 0 is the off position for a cap, on either axis
  it("treats 0 as no cap", () => {
    expect(isPagedUrlIndexable(999, true, caps(0, 0))).toBe(true);
    expect(isPagedUrlIndexable(999, false, caps(0, 0))).toBe(true);
  });
});

describe("evaluateFilterIndexability", () => {
  const facets = [
    facet("brand", [bucket("atelier", { value: "Atelier", description: "Handmade." }), bucket("hidden", { isIndexable: false })]),
    facet("size", [bucket("42", { value: "42" })], "AFTER"),
  ];

  it("indexes an unfiltered page and resolves nothing", () => {
    expect(evaluateFilterIndexability([], facets)).toEqual({ indexable: true, values: [] });
  });

  it("resolves a bucket to its display copy", () => {
    const out = evaluateFilterIndexability([{ slug: "brand", values: ["atelier"] }], facets);
    expect(out.indexable).toBe(true);
    expect(out.values).toEqual([{ display: "Atelier", position: "BEFORE", description: "Handmade." }]);
  });

  it("carries the group’s h1 position", () => {
    const out = evaluateFilterIndexability([{ slug: "size", values: ["42"] }], facets);
    expect(out.values[0].position).toBe("AFTER");
  });

  // one bad bucket is enough - the page as a whole is what gets indexed
  it("drops the whole page to noindex on a single non-indexable bucket", () => {
    const out = evaluateFilterIndexability(
      [{ slug: "brand", values: ["atelier", "hidden"] }],
      facets,
    );
    expect(out.indexable).toBe(false);
    expect(out.values).toHaveLength(2);
  });

  it("noindexes a bucket the facets do not know, and falls back to the raw slug", () => {
    const out = evaluateFilterIndexability([{ slug: "brand", values: ["ghost"] }], facets);
    expect(out.indexable).toBe(false);
    expect(out.values[0].display).toBe("ghost");
  });

  it("noindexes an attribute the facets do not know at all", () => {
    const out = evaluateFilterIndexability([{ slug: "nope", values: ["x"] }], facets);
    expect(out.indexable).toBe(false);
    expect(out.values[0].position).toBe("BEFORE");
  });

  // Multi-facet chains ride the seo/facets/indexable_combinations allowlist - unlisted
  // pairs are the 2^n crawl loop, listed ones are deliberate landing pages
  it("noindexes a facet combination that is not listed", () => {
    const out = evaluateFilterIndexability(
      [
        { slug: "brand", values: ["atelier"] },
        { slug: "size", values: ["42"] },
      ],
      facets,
    );
    expect(out.indexable).toBe(false);
  });

  it("indexes a listed combination, order-insensitively", () => {
    const rules = { combinations: [["brand", "size"]] };
    const chains = [
      [
        { slug: "brand", values: ["atelier"] },
        { slug: "size", values: ["42"] },
      ],
      [
        { slug: "size", values: ["42"] },
        { slug: "brand", values: ["atelier"] },
      ],
    ];
    for (const chain of chains) {
      expect(evaluateFilterIndexability(chain, facets, rules).indexable).toBe(true);
    }
  });

  it("keeps a listed combination noindex when one of its values is not indexable", () => {
    const rules = { combinations: [["brand", "size"]] };
    const out = evaluateFilterIndexability(
      [
        { slug: "brand", values: ["hidden"] },
        { slug: "size", values: ["42"] },
      ],
      facets,
      rules,
    );
    expect(out.indexable).toBe(false);
  });

  // OR pages duplicate their single-value parents, listed or not
  it("noindexes a multi-value pick of one facet even inside a listed combination", () => {
    const rules = { combinations: [["brand", "size"]] };
    const out = evaluateFilterIndexability(
      [
        { slug: "brand", values: ["atelier", "hidden"] },
        { slug: "size", values: ["42"] },
      ],
      facets,
      rules,
    );
    expect(out.indexable).toBe(false);
  });
});

describe("assembleH1", () => {
  const values = (...vs: { display: string; position: "BEFORE" | "AFTER" }[]) =>
    ({ indexable: true, values: vs.map((v) => ({ ...v, description: "" })) });

  it("wraps the base name with its before and after words", () => {
    expect(assembleH1("Shoes", values({ display: "Atelier", position: "BEFORE" }, { display: "42", position: "AFTER" }))).toBe(
      "Atelier Shoes 42",
    );
  });

  it("keeps the base name alone when nothing resolved", () => {
    expect(assembleH1("Shoes", values())).toBe("Shoes");
  });

  // a noindex page is not a canonical heading, so it must not advertise a filtered one
  it("keeps the base name alone on a noindex page", () => {
    expect(assembleH1("Shoes", { indexable: false, values: [{ display: "Atelier", position: "BEFORE", description: "" }] })).toBe("Shoes");
  });
});
