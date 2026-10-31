import { describe, expect, it } from "vitest";

import { ALL_GRANTED, NONE_GRANTED, parseConsent, resolveConsentCookie, serializeConsent } from "@/app/lib/consent/categories";

describe("consent cookie", () => {
  it("reads the old all/essential choice without asking again", () => {
    expect(parseConsent("all")).toEqual(ALL_GRANTED);
    expect(parseConsent("essential")).toEqual(NONE_GRANTED);
  });

  it("round-trips every category on its own", () => {
    const mixed = { analytics: true, ads: false, personalization: true };
    expect(serializeConsent(mixed)).toBe("v2:a1.d0.p1");
    expect(parseConsent(serializeConsent(mixed))).toEqual(mixed);
  });

  it("treats a missing or unknown value as no choice, never a grant", () => {
    for (const raw of [undefined, null, "", "yes", "v2:a1.d1", "v2:a2.d1.p1", "v3:a1.d1.p1"]) {
      expect(parseConsent(raw)).toBeNull();
    }
  });
});

describe("the store's consent mode", () => {
  it("opt-in leaves a visitor's choice alone and asks anyone without one", () => {
    expect(resolveConsentCookie(undefined, "opt_in", false)).toBeUndefined();
    expect(resolveConsentCookie("v2:a1.d0.p0", "opt_in", true)).toBeUndefined();
  });

  it("opt-in drops a default the store set while it was opt-out, so that visitor is asked", () => {
    expect(resolveConsentCookie("auto:a1.d1.p1", "opt_in", false)).toBe("");
  });

  it("opt-out turns everything on for a visitor with no choice, advertising off under Global Privacy Control", () => {
    expect(resolveConsentCookie(undefined, "opt_out", false)).toBe("auto:a1.d1.p1");
    expect(resolveConsentCookie(undefined, "opt_out", true)).toBe("auto:a1.d0.p1");
    expect(parseConsent("auto:a1.d0.p1")).toEqual({ analytics: true, ads: false, personalization: true });
  });

  it("opt-out keeps a visitor's own opt-out, and Global Privacy Control still overrides an advertising grant", () => {
    expect(resolveConsentCookie("v2:a0.d0.p0", "opt_out", false)).toBeUndefined();
    expect(resolveConsentCookie("v2:a1.d1.p1", "opt_out", true)).toBe("v2:a1.d0.p1");
    expect(resolveConsentCookie("auto:a1.d1.p1", "opt_out", true)).toBe("auto:a1.d0.p1");
  });
});
