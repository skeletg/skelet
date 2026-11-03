import { describe, expect, it } from "vitest";

import { isChannelTreePath } from "./channel-routing";

const CH = "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b";

describe("isChannelTreePath", () => {
  it("refuses a path that names a channel tree directly, file-looking tails and uppercase included", () => {
    expect(isChannelTreePath(`/${CH}/en/x.y`)).toBe(true);
    expect(isChannelTreePath(`/${CH}/en/products/shoe.png`)).toBe(true);
    expect(isChannelTreePath(`/${CH.toUpperCase()}/en`)).toBe(true);
    expect(isChannelTreePath(`/${CH}`)).toBe(true);
  });

  it("leaves pages, assets, api routes and sitemaps alone", () => {
    for (const p of ["/", "/en/products/shoe", "/_next/static/chunk.js", "/api/healthz", "/sitemap-index.xml", "/logo.png", `/p/${CH}`, `/${CH}.png`]) {
      expect(isChannelTreePath(p), p).toBe(false);
    }
  });
});
