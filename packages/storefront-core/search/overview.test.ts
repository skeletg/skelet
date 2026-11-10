import { describe, expect, it } from "vitest";

import { overviewExchange, overviewNamed, overviewPlainText, overviewSegments, searchedOnSite } from "./overview";

const dress = { id: "p1", name: "Smocked Midi Dress", url: "shop/dresses/smocked-midi-dress" };
const skirt = { id: "p2", name: "Linen Wrap Skirt", url: "shop/skirts/linen-wrap-skirt" };
const tee = { id: "p3", name: "Slub Tee", url: "shop/t-shirts/slub-tee" };

describe("overviewSegments", () => {
  it("turns each marker into the product it points at, with its position", () => {
    expect(overviewSegments("[[1]] is airy, and [[2]] ties at the waist.", [dress, skirt])).toEqual([
      { kind: "product", product: dress, position: 1 },
      { kind: "text", text: " is airy, and " },
      { kind: "product", product: skirt, position: 2 },
      { kind: "text", text: " ties at the waist." },
    ]);
  });

  it("drops markers that point nowhere without leaving a gap behind", () => {
    const segments = overviewSegments("Try [[7]] today, or [[0]]. Then [[2]] or [[x]].", [dress, skirt]);
    expect(segments).toEqual([
      { kind: "text", text: "Try today, or. Then " },
      { kind: "product", product: skirt, position: 2 },
      { kind: "text", text: " or [[x]]." },
    ]);
    expect(overviewSegments("Airy. [[4]]", [dress]).at(-1)).toEqual({ kind: "text", text: "Airy." });
    expect(overviewSegments("[[1]] [[4]]", [dress])).toEqual([{ kind: "product", product: dress, position: 1 }]);
  });

  it("keeps plain text whole and an empty text empty", () => {
    expect(overviewSegments("Nothing to link here.", [dress])).toEqual([{ kind: "text", text: "Nothing to link here." }]);
    expect(overviewSegments("", [dress])).toEqual([]);
  });
});

describe("overviewNamed", () => {
  it("lists the products the text names once each, in the order it names them", () => {
    const segments = overviewSegments("[[3]] first, then [[1]], and [[3]] again.", [dress, skirt, tee]);
    expect(overviewNamed(segments, [dress, skirt, tee])).toEqual([tee, dress]);
  });

  it("falls back to every product when the text names none", () => {
    expect(overviewNamed(overviewSegments("All good picks.", [dress, skirt]), [dress, skirt])).toEqual([dress, skirt]);
  });
});

describe("overviewExchange", () => {
  it("is the search as the question, the overview in plain words as the reply, and the backend's recap", () => {
    const recap = "Shown as cards: Smocked Midi Dress (id p1); Slub Tee (id p3)";
    const overview = { text: "For a summer wedding, [[1]] is airy and [[3]] is relaxed. [[9]]", recap, products: [dress, skirt, tee] };
    expect(overviewExchange("  breezy wedding  ", overview)).toEqual({
      question: "breezy wedding",
      reply: "For a summer wedding, Smocked Midi Dress is airy and Slub Tee is relaxed.",
      recap,
      productIds: ["p1", "p3"],
    });
    expect(overviewPlainText(overviewSegments("[[2]].", [dress, skirt]))).toBe("Linen Wrap Skirt.");
  });

  it("recaps nothing when there were no products", () => {
    expect(overviewExchange("q", { text: "Hm.", recap: "", products: [] })).toEqual({ question: "q", reply: "Hm.", recap: "", productIds: [] });
  });
});

describe("searchedOnSite", () => {
  const from = (h: Record<string, string>) => searchedOnSite(new Headers(h));

  it("takes a search made on the store's own pages", () => {
    expect(from({ "sec-fetch-site": "same-origin", host: "shop.example" })).toBe(true);
    expect(from({ referer: "https://shop.example/de/shop/dresses", host: "shop.example" })).toBe(true);
    expect(from({ referer: "https://shop.example/", "x-forwarded-host": "shop.example", host: "app:3000" })).toBe(true);
  });

  it("refuses a page landed on directly, from another site, or by something that doesn't say", () => {
    expect(from({ "sec-fetch-site": "none", host: "shop.example" })).toBe(false);
    expect(from({ "sec-fetch-site": "cross-site", referer: "https://www.google.com/", host: "shop.example" })).toBe(false);
    expect(from({ "sec-fetch-site": "same-site", host: "shop.example" })).toBe(false);
    expect(from({ referer: "https://elsewhere.example/search", host: "shop.example" })).toBe(false);
    expect(from({ host: "shop.example" })).toBe(false);
    expect(from({ referer: "not a url", host: "shop.example" })).toBe(false);
  });

  it("believes the browser over a referer it could be handed", () => {
    expect(from({ "sec-fetch-site": "none", referer: "https://shop.example/", host: "shop.example" })).toBe(false);
  });
});
