import { describe, expect, it } from "vitest";

import { parseDocument } from "@/app/components/cms/blocks";

const hero = { id: "a", type: "hero", props: { title: "Hi" } };

describe("parseDocument", () => {
  it("accepts a well-formed document and defaults missing props", () => {
    const doc = parseDocument({ version: 1, blocks: [hero, { id: "b", type: "divider" }] });
    expect(doc?.blocks.map((b) => b.id)).toEqual(["a", "b"]);
    expect(doc?.blocks[1].props).toEqual({});
  });

  it("accepts nested children that are well-formed", () => {
    const doc = parseDocument({ version: 1, blocks: [{ id: "s", type: "section", props: {}, blocks: [hero] }] });
    expect(doc?.blocks[0].blocks?.[0].id).toBe("a");
  });

  it("treats freeform bodies as no document", () => {
    expect(parseDocument(null)).toBeNull();
    expect(parseDocument("text")).toBeNull();
    expect(parseDocument([hero])).toBeNull();
    expect(parseDocument({ root: { children: [] } })).toBeNull();
  });

  it("requires an integer version", () => {
    expect(parseDocument({ blocks: [hero] })).toBeNull();
    expect(parseDocument({ version: "1", blocks: [hero] })).toBeNull();
    expect(parseDocument({ version: 1.5, blocks: [hero] })).toBeNull();
  });

  it("rejects the whole document when any node is malformed", () => {
    const html = { id: "x", type: "html", props: { html: "<img src=x onerror=alert(1)>" } };
    for (const bad of [
      { id: 1, type: "hero" },
      { type: "hero" },
      { id: "c", type: "" },
      { id: "c", type: "hero", props: [] },
      { id: "c", type: "hero", props: "x" },
      "hero",
      null,
    ]) {
      expect(parseDocument({ version: 1, blocks: [html, bad] })).toBeNull();
    }
  });

  it("rejects malformed children and over-deep trees", () => {
    expect(parseDocument({ version: 1, blocks: [{ id: "s", type: "section", blocks: [{ id: 2 }] }] })).toBeNull();
    expect(parseDocument({ version: 1, blocks: [{ id: "s", type: "section", blocks: "nope" }] })).toBeNull();
    let node: Record<string, unknown> = { id: "leaf", type: "hero" };
    for (let i = 0; i < 8; i++) node = { id: `n${i}`, type: "section", blocks: [node] };
    expect(parseDocument({ version: 1, blocks: [node] })).toBeNull();
  });
});
