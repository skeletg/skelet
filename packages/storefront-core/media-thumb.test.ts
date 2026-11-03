import { describe, expect, it } from "vitest";

import { mediaSrcSet, mediaThumb } from "./media-thumb";

const KEY = "catalog/products/abc/gallery/def.jpg";

describe("mediaThumb", () => {
  it("builds a sized URL from a raw asset URL", () => {
    expect(mediaThumb(`https://cdn.example.com/${KEY}`, 96)).toBe(
      `/media/t/w96/${KEY}`,
    );
  });

  it("clamps to the source width rather than upscaling", () => {
    expect(mediaThumb(`https://cdn.example.com/${KEY}`, 800, 500)).toBe(
      `/media/t/w500/${KEY}`,
    );
  });

  // The backend hands cart lines a w160 derivative, so re-sizing one naively nests the
  // prefix into /media/t/w96/media/t/w160/<key> - a key no bucket has, i.e. a 404
  it("re-sizes an already-transformed URL without nesting the prefix", () => {
    expect(mediaThumb(`/media/t/w160/${KEY}`, 96)).toBe(`/media/t/w96/${KEY}`);
  });

  it("also unwraps a width+height spec", () => {
    expect(mediaThumb(`/media/t/w160h120/${KEY}`, 96)).toBe(
      `/media/t/w96/${KEY}`,
    );
  });

  it("is idempotent under repeated application", () => {
    const once = mediaThumb(`https://cdn.example.com/${KEY}`, 96);
    expect(mediaThumb(once, 96)).toBe(once);
  });
});

describe("mediaSrcSet", () => {
  it("unwraps an already-transformed URL for every rung", () => {
    const set = mediaSrcSet(`/media/t/w160/${KEY}`);
    expect(set).toBeDefined();
    expect(set).not.toContain("media/t/w160/media");
    for (const rung of set!.split(", ")) {
      expect(rung).toMatch(/^\/media\/t\/w\d+\/catalog\/products\/abc\/gallery\/def\.jpg \d+w$/);
    }
  });

  it("drops rungs wider than the source", () => {
    expect(mediaSrcSet(`https://cdn.example.com/${KEY}`, 300)).toBe(
      `/media/t/w280/${KEY} 280w`,
    );
  });
});
