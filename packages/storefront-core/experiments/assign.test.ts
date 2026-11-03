import { describe, expect, it } from "vitest";

import { assign, type Experiment } from "./assign";

const exp = (over: Partial<Experiment> = {}): Experiment => ({
  key: "hero",
  coverage: 100,
  goalEvent: "purchase",
  variations: [
    { key: "control", weight: 50, value: "A" },
    { key: "variant", weight: 50, value: "B" },
  ],
  ...over,
});

// The rule exactly as experiments/client.ts and server.ts each carried it before it was shared (git HEAD) - live
// visitors were bucketed by this, so any drift would silently move them to another arm mid-experiment
function legacyAssign(e: Experiment, anon: string) {
  const fnv = (s: string) => {
    let h = 2166136261;
    for (let i = 0; i < s.length; i++) {
      h ^= s.charCodeAt(i);
      h = Math.imul(h, 16777619);
    }
    return h >>> 0;
  };
  const bucket = (seed: string) => (fnv(seed + anon) % 10000) / 10000;
  if (e.coverage <= 0 || e.variations.length === 0) return null;
  if (e.coverage < 100 && bucket(`${e.key}:coverage`) >= e.coverage / 100) return null;
  const total = e.variations.reduce((a, v) => a + Math.max(0, v.weight), 0);
  if (total <= 0) return e.variations[0];
  const target = bucket(e.key) * total;
  let acc = 0;
  for (const v of e.variations) {
    acc += Math.max(0, v.weight);
    if (target < acc) return v;
  }
  return e.variations[e.variations.length - 1];
}

const visitors = Array.from({ length: 4000 }, (_, i) => `anon-${i}`);

describe("assign", () => {
  it("assigns every visitor exactly as the pre-refactor client and server copies did", () => {
    const configs: Experiment[] = [
      exp(),
      exp({ key: "checkout-cta", coverage: 37 }),
      exp({
        key: "three-way",
        coverage: 90,
        variations: [
          { key: "a", weight: 1, value: "A" },
          { key: "b", weight: 2, value: "B" },
          { key: "c", weight: 7, value: "C" },
        ],
      }),
      exp({ key: "negatives", variations: [{ key: "x", weight: -1, value: "X" }, { key: "y", weight: 3, value: "Y" }] }),
    ];
    for (const c of configs) {
      for (const v of visitors) expect(assign(c, v)).toBe(legacyAssign(c, v));
    }
  });

  it("puts a visitor in the same arm every time, so client and server agree", () => {
    for (const v of visitors.slice(0, 200)) {
      expect(assign(exp(), v)).toEqual(assign(exp(), v));
    }
  });

  it("enters nobody at zero coverage and nobody into an experiment with no arms", () => {
    expect(assign(exp({ coverage: 0 }), "anon-1")).toBeNull();
    expect(assign(exp({ variations: [] }), "anon-1")).toBeNull();
  });

  it("enters roughly the coverage share of visitors", () => {
    const entered = visitors.filter((v) => assign(exp({ coverage: 25 }), v) !== null).length;
    expect(entered / visitors.length).toBeGreaterThan(0.2);
    expect(entered / visitors.length).toBeLessThan(0.3);
  });

  it("splits entered visitors by weight", () => {
    const e = exp({
      variations: [
        { key: "control", weight: 80, value: "A" },
        { key: "variant", weight: 20, value: "B" },
      ],
    });
    const variant = visitors.filter((v) => assign(e, v)?.key === "variant").length;
    expect(variant / visitors.length).toBeGreaterThan(0.15);
    expect(variant / visitors.length).toBeLessThan(0.25);
  });

  it("never picks a zero-weight arm", () => {
    const e = exp({
      variations: [
        { key: "control", weight: 100, value: "A" },
        { key: "off", weight: 0, value: "B" },
      ],
    });
    expect(visitors.some((v) => assign(e, v)?.key === "off")).toBe(false);
  });

  it("falls back to the first arm when every weight is zero or negative", () => {
    const e = exp({
      variations: [
        { key: "first", weight: 0, value: "A" },
        { key: "second", weight: -5, value: "B" },
      ],
    });
    expect(assign(e, "anon-1")?.key).toBe("first");
  });

  it("buckets per experiment key, so two experiments don't split visitors identically", () => {
    const a = visitors.map((v) => assign(exp({ key: "one" }), v)?.key);
    const b = visitors.map((v) => assign(exp({ key: "two" }), v)?.key);
    expect(a).not.toEqual(b);
  });
});
