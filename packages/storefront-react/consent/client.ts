"use client";

import { CONSENT_COOKIE } from "@skelet/storefront-core/consent/cookie";
import { NONE_GRANTED, parseConsent, type Consent } from "@skelet/storefront-core/consent/categories";

export type ClientConsent = Consent & { decided: boolean };

// httpOnly is false on purpose, so a cached page can resolve consent without a server render
export function readClientConsent(): ClientConsent {
  if (typeof document === "undefined") return { ...NONE_GRANTED, decided: false };
  const hit = document.cookie
    .split("; ")
    .find((row) => row.startsWith(`${CONSENT_COOKIE}=`));
  const c = parseConsent(hit ? decodeURIComponent(hit.slice(CONSENT_COOKIE.length + 1)) : null);
  return { ...(c ?? NONE_GRANTED), decided: c !== null };
}

// Whether the server-side destinations may hear from this visitor at all - each needs its own category, which the
// backend checks per destination, so any grant is reason enough to send
export function trackingAllowed(): boolean {
  const c = readClientConsent();
  return c.analytics || c.ads || c.personalization;
}
