import { describe, expect, it } from "vitest";

import { buildForwardedCookie } from "@/app/lib/forwarded-cookie";

describe("buildForwardedCookie", () => {
  it("replaces an existing cookie instead of appending a duplicate", () => {
    const out = buildForwardedCookie("access=stale; other=1", { access: "fresh" });
    expect(out).toBe("other=1; access=fresh");
  });

  it("appends when the cookie is absent", () => {
    expect(buildForwardedCookie("other=1", { access: "fresh" })).toBe("other=1; access=fresh");
  });

  it("handles a missing cookie header", () => {
    expect(buildForwardedCookie(null, { access: "fresh" })).toBe("access=fresh");
  });

  it("does not confuse prefix-colliding names", () => {
    const out = buildForwardedCookie("access_backup=keep; access=stale", { access: "fresh" });
    expect(out).toContain("access_backup=keep");
    expect(out).toContain("access=fresh");
    expect(out).not.toContain("access=stale");
  });

  it("keeps values containing an equals sign intact", () => {
    const out = buildForwardedCookie("jwt=header.payload=sig; other=1", { other: "2" });
    expect(out).toContain("jwt=header.payload=sig");
    expect(out).toContain("other=2");
  });

  it("survives sloppy spacing and empty segments", () => {
    const out = buildForwardedCookie("  a=1 ;; b=2 ; ", { c: "3" });
    expect(out).toBe("a=1; b=2; c=3");
  });
});
