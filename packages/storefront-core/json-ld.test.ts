import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  breadcrumbJsonLd,
  itemListJsonLd,
  organizationJsonLd,
  parseCustomJsonLd,
  productJsonLd,
  webSiteJsonLd,
} from "./json-ld";

// Google reads this, and it fails closed in the worst way: malformed structured data does not break
// the page, it just quietly stops earning rich results. So the checks are about shape - money as
// major units, absolute urls, and no half-built aggregateRating

const product = (over: Partial<Parameters<typeof productJsonLd>[0]> = {}) => ({
  id: "p1",
  url: "/products/boot",
  sku: "BOOT-1",
  name: "Chelsea Boot",
  defaultPrice: 12995,
  defaultCurrencyCode: "USD",
  inStock: true,
  ...over,
});

const offers = (out: Record<string, unknown>) => out.offers as Record<string, unknown>;

// displayName warns on a missing translation, which is the point of it - keep it out of the output
beforeEach(() => {
  vi.spyOn(console, "warn").mockImplementation(() => {});
});

describe("productJsonLd", () => {
  it("describes the product", () => {
    const out = productJsonLd(product(), "/products/boot");
    expect(out["@type"]).toBe("Product");
    expect(out.name).toBe("Chelsea Boot");
    expect(out.sku).toBe("BOOT-1");
    expect(out.productID).toBe("p1");
  });

  // the backend stores minor units; schema.org wants major, and 129.95 vs 12995 is a wildly
  // wrong price sitting in a search result
  it("converts the price out of minor units, with both decimals", () => {
    expect(offers(productJsonLd(product(), "/p")).price).toBe("129.95");
    expect(offers(productJsonLd(product({ defaultPrice: 12000 }), "/p")).price).toBe("120.00");
    expect(offers(productJsonLd(product({ defaultPrice: 5 }), "/p")).price).toBe("0.05");
  });

  it("uses the currency's precision, not a fixed hundred", () => {
    expect(offers(productJsonLd(product({ defaultPrice: 1500, defaultCurrencyCode: "JPY" }), "/p")).price).toBe("1500");
    expect(offers(productJsonLd(product({ defaultPrice: 1500, defaultCurrencyCode: "BHD" }), "/p")).price).toBe("1.500");
    const configured = { code: "USD", symbol: "$", symbolPosition: "prefix", decimals: 3 };
    expect(offers(productJsonLd(product(), "/p", undefined, undefined, configured)).price).toBe("12.995");
  });

  it("maps stock onto the schema.org enumeration", () => {
    expect(offers(productJsonLd(product(), "/p")).availability).toBe("https://schema.org/InStock");
    expect(offers(productJsonLd(product({ inStock: false }), "/p")).availability).toBe("https://schema.org/OutOfStock");
  });

  it("makes every url absolute, since a relative one is not a valid identifier", () => {
    const out = productJsonLd(product(), "/products/boot");
    expect(String(out.url)).toMatch(/^https?:\/\/.+\/products\/boot$/);
    expect(out["@id"]).toBe(out.url);
    expect(offers(out).url).toBe(out.url);
  });

  // deliberately NOT the sku - displayName refuses to render a slug as a name, because a passable
  // looking identifier in a search result hides that the product has no translation
  it("marks an untranslated product rather than leaking its sku", () => {
    expect(productJsonLd(product({ name: null }), "/p").name).toBe("(unnamed)");
  });

  // absolute, main image first, no repeats - a relative image URL is meaningless to a
  // crawler holding the JSON outside the page
  it("absolutizes images, main image first, without repeats", () => {
    const out = productJsonLd(
      product({ mainImage: { url: "/a.jpg" }, media: [{ url: "/a.jpg" }, { url: "/b.jpg" }] }),
      "/p",
    );
    const images = out.image as string[];
    expect(images).toHaveLength(2);
    expect(images[0]).toMatch(/^https?:\/\/.+\/a\.jpg$/);
    expect(images[1]).toMatch(/^https?:\/\/.+\/b\.jpg$/);
  });

  it("leaves image out entirely rather than emitting an empty list", () => {
    expect(productJsonLd(product(), "/p").image).toBeUndefined();
  });

  it("prefers the operator's meta description over the body copy", () => {
    const out = productJsonLd(product({ metaDescription: "Operator copy" }), "/p");
    expect(out.description).toBe("Operator copy");
  });

  it("omits description when there is nothing to say", () => {
    expect(productJsonLd(product(), "/p").description).toBeUndefined();
  });

  // Brand and identifiers live in ordinary catalog attributes under conventional slugs - a store
  // without them just omits the fields, which Google treats as a warning, not an error
  it("lifts brand, gtin and mpn out of the conventional attributes", () => {
    const out = productJsonLd(
      product({
        attributeValues: [
          { attribute: { slug: "brand" }, selectedValues: [{ name: "Acme", value: "acme" }] },
          { attribute: { slug: "ean" }, selectedValues: [], scalarValue: "4006381333931" },
          { attribute: { slug: "mpn" }, selectedValues: [], scalarValue: "AC-1234" },
        ],
      }),
      "/p",
    );
    expect(out.brand).toEqual({ "@type": "Brand", name: "Acme" });
    expect(out.gtin).toBe("4006381333931");
    expect(out.mpn).toBe("AC-1234");
  });

  it("omits brand rather than inventing one", () => {
    expect(productJsonLd(product(), "/p").brand).toBeUndefined();
  });

  // A configurable's price is whatever its variants cost - a single Offer at the anchor price
  // advertises a number nobody can pay
  it("aggregates a configurable's offer across its variants", () => {
    const out = productJsonLd(
      product({
        kind: "CONFIGURABLE",
        inStock: false,
        children: [
          { defaultPrice: 9900, inStock: false },
          { defaultPrice: 14900, inStock: true },
        ],
      }),
      "/p",
    );
    expect(offers(out)).toMatchObject({
      "@type": "AggregateOffer",
      lowPrice: "99.00",
      highPrice: "149.00",
      offerCount: 2,
      availability: "https://schema.org/InStock", // any purchasable variant = in stock
    });
  });

  it("keeps the plain offer for a simple product even if children leak in", () => {
    const out = productJsonLd(
      product({ children: [{ defaultPrice: 1, inStock: true }] }),
      "/p",
    );
    expect(offers(out)["@type"]).toBe("Offer");
  });
});

describe("productJsonLd with reviews", () => {
  const reviews = (over: Partial<Parameters<typeof productJsonLd>[2] & object> = {}) => ({
    count: 2,
    overallAverage: 4.5,
    reviews: [
      { customerDisplayName: "Ada", title: "Great", body: "Very good", submittedAt: "2026-01-01T00:00:00Z", overallRating: 5 },
      { customerDisplayName: "Bob", title: null, body: null, submittedAt: "2026-01-02T00:00:00Z", overallRating: 4 },
    ],
    ...over,
  });

  it("adds an aggregate rating and the individual reviews", () => {
    const out = productJsonLd(product(), "/p", reviews());
    expect(out.aggregateRating).toMatchObject({ ratingValue: 4.5, reviewCount: 2, bestRating: 5, worstRating: 1 });
    expect(out.review).toHaveLength(2);
  });

  // Google rejects an aggregateRating with no rating, so a product with reviews but no average
  // must not advertise one
  it("adds nothing when there is no average to report", () => {
    expect(productJsonLd(product(), "/p", reviews({ overallAverage: null })).aggregateRating).toBeUndefined();
    expect(productJsonLd(product(), "/p", reviews({ count: 0 })).aggregateRating).toBeUndefined();
  });

  it("rounds a long average rather than emitting its full float", () => {
    const out = productJsonLd(product(), "/p", reviews({ overallAverage: 4.666666 }));
    expect((out.aggregateRating as Record<string, unknown>).ratingValue).toBe(4.67);
  });

  it("skips a review with no rating, which cannot become a Rating node", () => {
    const out = productJsonLd(product(), "/p", reviews({
      reviews: [{ customerDisplayName: "Ada", submittedAt: "2026-01-01T00:00:00Z", overallRating: null }],
    }));
    expect(out.review).toBeUndefined()
    expect(out.aggregateRating).toBeDefined() // the average still stands
  });

  it("caps the listed reviews at ten", () => {
    const many = Array.from({ length: 25 }, (_, i) => ({
      customerDisplayName: `R${i}`, title: null, body: null, submittedAt: "2026-01-01T00:00:00Z", overallRating: 5,
    }));
    expect(productJsonLd(product(), "/p", reviews({ reviews: many })).review).toHaveLength(10);
  });

  it("names an anonymous reviewer rather than emitting a blank author", () => {
    const out = productJsonLd(product(), "/p", reviews({
      reviews: [{ customerDisplayName: "", title: null, body: null, submittedAt: "2026-01-01T00:00:00Z", overallRating: 5 }],
    }));
    const first = (out.review as Record<string, unknown>[])[0]
    expect((first.author as Record<string, unknown>).name).toBe("Customer");
  });
})

describe("itemListJsonLd", () => {
  const items = [
    { url: "boots/chelsea", sku: "B-1", name: "Chelsea Boot" },
    { url: "boots/combat", sku: "B-2", name: null },
  ];
  const toHref = (p: { url: string }) => "/" + p.url;

  it("lists each product as an absolute pointer at its PDP", () => {
    const out = itemListJsonLd(items, toHref);
    expect(out["@type"]).toBe("ItemList");
    expect(out.numberOfItems).toBe(2);
    const els = out.itemListElement as Record<string, unknown>[];
    expect(els[0]).toMatchObject({ "@type": "ListItem", position: 1, name: "Chelsea Boot" });
    expect(String(els[0].url)).toMatch(/^https?:\/\/.+\/boots\/chelsea$/);
  });

  // page 2 of a 24-per-page list starts at 25, not at 1 - restarting positions makes
  // every page claim to be the top of the list
  it("continues positions across pagination", () => {
    const els = itemListJsonLd(items, toHref, 24).itemListElement as Record<string, unknown>[];
    expect(els.map((e) => e.position)).toEqual([25, 26]);
  });
});

describe("breadcrumbJsonLd", () => {
  it("numbers positions from one, as the spec requires", () => {
    const out = breadcrumbJsonLd([{ name: "Home", href: "/" }, { name: "Boots", href: "/boots" }]);
    const items = out.itemListElement as Record<string, unknown>[];
    expect(items.map((i) => i.position)).toEqual([1, 2]);
    expect(items[1].name).toBe("Boots");
  });

  it("absolutises every crumb", () => {
    const items = breadcrumbJsonLd([{ name: "Boots", href: "/boots" }]).itemListElement as Record<string, unknown>[];
    expect(String(items[0].item)).toMatch(/^https?:\/\/.+\/boots$/);
  });

  it("survives an empty trail", () => {
    expect(breadcrumbJsonLd([]).itemListElement).toEqual([]);
  });
});

describe("parseCustomJsonLd", () => {
  it("parses what the operator wrote", () => {
    expect(parseCustomJsonLd('{"@type":"FAQPage"}')).toEqual({ "@type": "FAQPage" });
  });

  // this is hand-typed into an admin field, so bad input has to become nothing at all rather
  // than a broken <script> tag on every page
  it("answers null for blank, missing or unparseable input", () => {
    expect(parseCustomJsonLd(null)).toBeNull();
    expect(parseCustomJsonLd("")).toBeNull();
    expect(parseCustomJsonLd("   ")).toBeNull();
    expect(parseCustomJsonLd("{ not json")).toBeNull();
  });
});

describe("site-level nodes", () => {
  it("gives the organization a stable id the website can point at", () => {
    const org = organizationJsonLd({ name: "Acme" });
    const site = webSiteJsonLd({ name: "Acme" });
    expect(String(org["@id"])).toMatch(/#organization$/);
    expect(site.publisher).toEqual({ "@id": org["@id"] });
  });

  it("includes an email only when there is one", () => {
    expect(organizationJsonLd({ name: "Acme" }).email).toBeUndefined();
    expect(organizationJsonLd({ name: "Acme", email: "hi@acme.test" }).email).toBe("hi@acme.test");
  });

  it("advertises search with the token Google substitutes into", () => {
    const action = webSiteJsonLd({ name: "Acme" }).potentialAction as Record<string, unknown>;
    expect(String((action.target as Record<string, unknown>).urlTemplate)).toContain("{search_term_string}");
    expect(action["query-input"]).toBe("required name=search_term_string");
  });
});
