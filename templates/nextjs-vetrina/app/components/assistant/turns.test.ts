import { describe, expect, it } from "vitest";

import { readChatId, readTurn, readTurns, toMessages, withExchange, type Exchange, type Turn } from "./turns";

const product = { id: "p1", name: "Mug", kind: "SIMPLE", defaultPrice: 1200 };
const link = { label: "Mugs", path: "/shop/mugs", query: "", filters: [{ slug: "color", values: ["red"] }], sort: "", count: 3 };

describe("readTurns", () => {
  it("comes back empty from nothing, noise, or the wrong shape", () => {
    for (const raw of [null, "", "{not json", "7", "null", "[]", JSON.stringify({ turns: "x" }), JSON.stringify([{ role: "user", content: "hi" }])]) {
      expect(readTurns(raw)).toEqual([]);
    }
  });

  it("keeps the turns it can draw and drops the rest", () => {
    const raw = JSON.stringify({
      turns: [{ role: "user", content: "mugs?" }, { role: "robot", content: "x" }, { role: "assistant", content: 7 }, null, "hi", { role: "assistant", content: "Two.", products: [product] }],
    });
    expect(readTurns(raw)).toEqual([
      { role: "user", content: "mugs?" },
      { role: "assistant", content: "Two.", products: [product] },
    ]);
  });
});

describe("readTurn", () => {
  it("keeps only what a user said", () => {
    expect(readTurn({ role: "user", content: "hi", products: [product], showCart: true })).toEqual({ role: "user", content: "hi" });
  });

  it("leaves off the parts of an older turn that no longer have the shape the panel draws", () => {
    const turn = readTurn({
      role: "assistant",
      content: "Here",
      recap: 7,
      followUps: ["more?", 3],
      detail: { product, specs: [], notes: { summary: "x" } },
      links: [link, { label: "Old", url: "/x" }],
      navigated: { ...link, count: "3" },
      products: [{ id: "p2" }],
      showCart: "yes",
      incomplete: 1,
    });
    expect(turn).toEqual({ role: "assistant", content: "Here" });
  });

  it("keeps a full reply as it was stored", () => {
    const detail = { product, specs: [{ label: "Size", value: "M" }], notes: { summary: "", highlights: [{ title: "Sturdy", detail: "" }], goodFor: [], keepInMind: [] } };
    const stored = { role: "assistant", content: "Here", recap: "r", followUps: ["more?"], detail, links: [link], navigated: link, products: [product], showCart: true, incomplete: true };
    expect(readTurn(stored)).toEqual(stored);
  });

  it("drops links that would leave the store", () => {
    expect(readTurn({ role: "assistant", content: "x", links: [{ ...link, path: "//evil.example" }], navigated: { ...link, path: "/\\evil.example" } })).toEqual({ role: "assistant", content: "x" });
  });
});

describe("readChatId", () => {
  it("keeps a saved chat id and makes a new one otherwise", () => {
    const id = "0f6c7a52-6f3e-4b55-9d8e-2c1b8f0e9a11";
    expect(readChatId(JSON.stringify({ id }))).toBe(id);
    for (const raw of [null, "{oops", JSON.stringify({ id: "'; drop" }), JSON.stringify({ turns: [] })]) {
      expect(readChatId(raw)).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
    }
  });
});

describe("withExchange", () => {
  const exchange: Exchange = { question: "summer wedding", reply: "Try the Mug.", recap: "Shown as cards: Mug (id p1)", products: [product] as unknown as Exchange["products"] };

  it("adds the question and its reply after what was said, and sends the reply back with its recap like any other", () => {
    const earlier: Turn[] = [{ role: "user", content: "hi" }, { role: "assistant", content: "Hello" }];
    const turns = withExchange(earlier, exchange);
    expect(turns).toEqual([...earlier, { role: "user", content: "summer wedding" }, { role: "assistant", content: "Try the Mug.", recap: "Shown as cards: Mug (id p1)", products: [product] }]);
    expect(toMessages(turns).slice(-2)).toEqual([
      { role: "user", content: "summer wedding" },
      { role: "assistant", content: "Try the Mug.", recap: "Shown as cards: Mug (id p1)" },
    ]);
    expect(readTurns(JSON.stringify({ turns }))).toEqual(turns);
  });

  it("doesn't say the same exchange twice in a row", () => {
    const once = withExchange([], exchange);
    expect(withExchange(once, exchange)).toBe(once);
    expect(withExchange([...once, { role: "user", content: "more?" }], exchange)).toHaveLength(5);
  });

  it("leaves off an empty recap and no cards", () => {
    expect(withExchange([], { question: "q", reply: "r", recap: "", products: [] })).toEqual([
      { role: "user", content: "q" },
      { role: "assistant", content: "r" },
    ]);
  });
});
