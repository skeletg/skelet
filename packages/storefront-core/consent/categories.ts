// The consent cookie's value, shared by server, browser and the edge proxy - no Next imports, so all three can load it.
// "v2:a1.d0.p0" = analytics, ads (Consent Mode's ad_storage + ad_user_data) and personalization, each 1 or 0. The
// pre-category values still parse: "all" grants everything and "essential" nothing, so nobody is asked again

export type Consent = { analytics: boolean; ads: boolean; personalization: boolean };

export const ALL_GRANTED: Consent = { analytics: true, ads: true, personalization: true };
export const NONE_GRANTED: Consent = { analytics: false, ads: false, personalization: false };

// "auto:" marks a value the store set under opt-out mode rather than one the visitor chose - switching the store back
// to opt-in drops it, so nobody counts as having agreed to something they were never asked
const V2 = /^(?:v2|auto):a([01])\.d([01])\.p([01])$/;

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

// The store's general/privacy/cookie_consent - opt_in asks first (EU/UK), opt_out asks nobody and lets them switch off (US)
export type ConsentMode = "opt_in" | "opt_out";

// What the proxy writes before anything reads consent: undefined leaves the cookie alone, "" deletes it. Under opt-out a
// visitor with no choice gets everything on, except advertising when their browser sends Global Privacy Control - which
// also overrides an earlier advertising grant, as the US state laws require. Under opt-in a store-set default is dropped
export function resolveConsentCookie(raw: string | undefined, mode: ConsentMode, gpc: boolean): string | undefined {
  const current = parseConsent(raw);
  if (mode === "opt_in") return raw?.startsWith("auto:") ? "" : undefined;
  if (!current) return "auto:" + serializeConsent({ ...ALL_GRANTED, ads: !gpc }).slice("v2:".length);
  if (gpc && current.ads) return (raw!.startsWith("auto:") ? "auto:" : "v2:") + serializeConsent({ ...current, ads: false }).slice("v2:".length);
  return undefined;
}
