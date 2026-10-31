// The consent cookie's value, shared by server, browser and the edge proxy - no Next imports, so all three can load it.
// "v2:a1.d0.p0" = analytics, ads (Consent Mode's ad_storage + ad_user_data) and personalization, each 1 or 0. The
// pre-category values still parse: "all" grants everything and "essential" nothing, so nobody is asked again

export type Consent = { analytics: boolean; ads: boolean; personalization: boolean };

export const ALL_GRANTED: Consent = { analytics: true, ads: true, personalization: true };
export const NONE_GRANTED: Consent = { analytics: false, ads: false, personalization: false };

const V2 = /^v2:a([01])\.d([01])\.p([01])$/;

// null = no choice made yet (or a value we don't recognise, which must not count as a grant)
export function parseConsent(raw: string | undefined | null): Consent | null {
  if (raw === "all") return ALL_GRANTED;
  if (raw === "essential") return NONE_GRANTED;
  const m = raw ? V2.exec(raw) : null;
  if (!m) return null;
  return { analytics: m[1] === "1", ads: m[2] === "1", personalization: m[3] === "1" };
}

export function serializeConsent(c: Consent): string {
  return `v2:a${+c.analytics}.d${+c.ads}.p${+c.personalization}`;
}
