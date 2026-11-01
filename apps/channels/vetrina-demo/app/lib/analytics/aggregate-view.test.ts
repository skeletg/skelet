import { describe, expect, it } from "vitest";

import { countableView } from "@/app/lib/analytics/aggregate-view";

const h = (init: Record<string, string> = {}) => new Headers(init);

describe("an aggregate page view", () => {
  it("counts a real look at a page by a visitor without analytics consent", () => {
    expect(countableView("GET", h(), false, false)).toBe(true);
    expect(countableView("GET", h({ rsc: "1" }), false, false)).toBe(true); // a client-side navigation
  });

  it("leaves out consenting visitors, whom Lens counts, and bots, prefetches and form posts", () => {
    expect(countableView("GET", h(), true, false)).toBe(false);
    expect(countableView("GET", h(), false, true)).toBe(false);
    expect(countableView("GET", h({ rsc: "1", "next-router-prefetch": "1" }), false, false)).toBe(false);
    expect(countableView("GET", h({ "sec-purpose": "prefetch;prerender" }), false, false)).toBe(false);
    expect(countableView("POST", h(), false, false)).toBe(false);
  });
});
