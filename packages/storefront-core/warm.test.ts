import { describe, expect, it } from "vitest";

import { warmUntilRendered } from "./warm";

const page = (cache: string | null, status = 200) =>
  new Response("", { status, headers: cache ? { "x-nextjs-cache": cache } : {} });

describe("warmUntilRendered", () => {
  it("keeps asking while the old cached page answers, until the purge lands and it renders", async () => {
    const answers = [page("HIT"), page("HIT"), page("MISS")];
    let calls = 0;
    const ok = await warmUntilRendered(async () => answers[calls++], { delayMs: 0 });
    expect(ok).toBe(true);
    expect(calls).toBe(3);
  });

  it("stops on the first render, and on a page that isn't cached at all", async () => {
    let calls = 0;
    expect(await warmUntilRendered(async () => (calls++, page("MISS")), { delayMs: 0 })).toBe(true);
    expect(await warmUntilRendered(async () => (calls++, page(null)), { delayMs: 0 })).toBe(true);
    expect(calls).toBe(2);
  });

  it("reports a failed render so the backend retries it", async () => {
    expect(await warmUntilRendered(async () => page(null, 500), { delayMs: 0 })).toBe(false);
  });

  it("gives up after its tries on a page that stays cached", async () => {
    let calls = 0;
    expect(await warmUntilRendered(async () => (calls++, page("HIT")), { tries: 4, delayMs: 0 })).toBe(true);
    expect(calls).toBe(4);
  });
});
