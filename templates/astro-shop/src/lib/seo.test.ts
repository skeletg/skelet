import { describe, expect, it } from "vitest";

import { absolute, breadcrumbLd, clip, jsonLd, productLd, siteOrigin, sitemapXml, websiteLd } from "./seo";

describe("siteOrigin", () => {
  it("prefers the channel's public URL and drops its path", () => {
    expect(siteOrigin("https://shop.example.com/en/", "http://localhost:3000")).toBe("https://shop.example.com");
  });

  it("falls back to the request origin when the public URL is missing or malformed", () => {
    expect(siteOrigin(null, "http://localhost:3000")).toBe("http://localhost:3000");
    expect(siteOrigin("not a url", "http://localhost:3000")).toBe("http://localhost:3000");
  });
});

describe("absolute", () => {
  it("joins with or without a leading slash", () => {
    expect(absolute("https://a.test", "mug")).toBe("https://a.test/mug");
    expect(absolute("https://a.test", "/mug")).toBe("https://a.test/mug");
  });
});

describe("jsonLd", () => {
  it("cannot close the surrounding script tag", () => {
    const out = jsonLd({ name: "</script><script>alert(1)</script>" });
    expect(out).not.toContain("</script>");
    expect(JSON.parse(out).name).toBe("</script><script>alert(1)</script>");
  });
});

describe("productLd", () => {
  const base = { name: "Mug", sku: "MUG-1", url: "https://a.test/mug", price: 1250, inStock: true };

  it("prices in major units with the currency's decimals", () => {
    const ld = productLd(base, { code: "USD", decimals: 2 }, "Shop");
    expect(ld.offers).toMatchObject({ price: "12.50", priceCurrency: "USD", availability: "https://schema.org/InStock" });
    expect(ld.brand).toEqual({ "@type": "Brand", name: "Shop" });
    expect(ld).not.toHaveProperty("image");
  });

  it("handles zero-decimal currencies and sold-out stock", () => {
    const ld = productLd({ ...base, price: 500, inStock: false, image: "https://a.test/m.jpg" }, { code: "JPY", decimals: 0 }, "Shop");
    expect(ld.offers.price).toBe("500");
    expect(ld.offers.availability).toBe("https://schema.org/OutOfStock");
    expect(ld.image).toEqual(["https://a.test/m.jpg"]);
  });
});

describe("breadcrumbLd and websiteLd", () => {
  it("numbers crumbs from one", () => {
    const ld = breadcrumbLd([
      { name: "Shop", url: "https://a.test/" },
      { name: "Mug", url: "https://a.test/mug" },
    ]);
    expect((ld.itemListElement as { position: number }[]).map((i) => i.position)).toEqual([1, 2]);
  });

  it("points the site at its root", () => {
    expect(websiteLd("Shop", "https://a.test").url).toBe("https://a.test/");
  });
});

describe("clip", () => {
  it("keeps short text and collapses whitespace", () => {
    expect(clip("  soft\n\n cotton  ")).toBe("soft cotton");
  });

  it("cuts long text on a word boundary under the limit", () => {
    const out = clip("word ".repeat(60));
    expect(out.length).toBeLessThanOrEqual(160);
    expect(out.endsWith("word…")).toBe(true);
  });
});

describe("sitemapXml", () => {
  it("lists every URL, escaped", () => {
    const xml = sitemapXml(["https://a.test/", "https://a.test/a&b"]);
    expect(xml).toContain("<loc>https://a.test/</loc>");
    expect(xml).toContain("<loc>https://a.test/a&amp;b</loc>");
    expect(xml.startsWith('<?xml version="1.0"')).toBe(true);
  });
});
