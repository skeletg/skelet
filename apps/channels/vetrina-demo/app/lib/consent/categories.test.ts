import { describe, expect, it } from "vitest";

import { ALL_GRANTED, NONE_GRANTED, parseConsent, serializeConsent } from "@/app/lib/consent/categories";

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
