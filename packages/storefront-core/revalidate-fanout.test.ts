import { describe, expect, it } from "vitest";

import { localesByChannel, mapLimit, warmAcrossChannels } from "./revalidate-fanout";

const tick = () => new Promise((r) => setTimeout(r, 1));

function tracked<T>(fn: (x: T) => unknown) {
  const state = { active: 0, peak: 0 };
  const run = async (x: T) => {
    state.active++;
    state.peak = Math.max(state.peak, state.active);
    await tick();
    state.active--;
    return fn(x);
  };
  return { state, run };
}

describe("mapLimit", () => {
  it("never runs more than the limit at once and keeps results in input order", async () => {
    const { state, run } = tracked((n: number) => n * 2);
    const out = await mapLimit([1, 2, 3, 4, 5, 6, 7], 3, async (n) => (await run(n)) as number);
    expect(state.peak).toBe(3);
    expect(out.map((r) => (r.status === "fulfilled" ? r.value : null))).toEqual([2, 4, 6, 8, 10, 12, 14]);
  });

  it("settles a throwing item without stopping the rest", async () => {
    const out = await mapLimit([1, 2, 3], 2, async (n) => {
      if (n === 2) throw new Error("boom");
      return n;
    });
    expect(out.map((r) => r.status)).toEqual(["fulfilled", "rejected", "fulfilled"]);
  });
});

describe("warmAcrossChannels", () => {
  it("bounds the fan-out and names every channel and path that failed, even when others warmed", async () => {
    const { state, run } = tracked((j: { ch: string; p: string }) => !(j.ch === "b" && j.p === "/x"));
    const channels = ["a", "b", "c", "d", "e"];
    const res = await warmAcrossChannels(channels, ["/", "/x"], async (ch, p) => (await run({ ch, p })) as boolean, 2);
    expect(state.peak).toBe(2);
    expect(res.warmed).toBe(9);
    expect(res.failed).toEqual([{ channel: "b", path: "/x" }]);
  });

  it("counts a warm that threw as failed", async () => {
    const res = await warmAcrossChannels(["a"], ["/"], async () => {
      throw new Error("timeout");
    });
    expect(res).toEqual({ warmed: 0, failed: [{ channel: "a", path: "/" }] });
  });
});

describe("localesByChannel", () => {
  it("loads channels concurrently within the bound, and an unreachable one gets no locales", async () => {
    const { state, run } = tracked((ch: string) => {
      if (ch === "bad") throw new Error("down");
      return { codes: [`${ch}-en`] };
    });
    const got = await localesByChannel(["a", "bad", "c", "d"], async (ch) => (await run(ch)) as { codes: string[] }, 3);
    expect(state.peak).toBe(3);
    expect(Object.fromEntries(got)).toEqual({ a: ["a-en"], bad: [], c: ["c-en"], d: ["d-en"] });
  });
});
