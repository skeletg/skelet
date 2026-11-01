import { describe, expect, it } from "vitest";

import { sessionWrite } from "@/app/lib/analytics/session";

describe("the analytics session cookie", () => {
  const now = 1_700_000_000_000;
  it("starts a session at the first page with analytics consent, and keeps it rolling", () => {
    expect(sessionWrite(undefined, true, now)).toBe("1700000000");
    expect(sessionWrite("1699999000", true, now)).toBe("1699999000");
  });
  it("replaces a value that isn't a session, and deletes it without analytics consent", () => {
    expect(sessionWrite("junk", true, now)).toBe("1700000000");
    expect(sessionWrite("1699999000", false, now)).toBe("");
    expect(sessionWrite(undefined, false, now)).toBeUndefined();
  });
});
