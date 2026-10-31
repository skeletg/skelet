"use client";

import { CONSENT_COOKIE } from "@/app/lib/consent/cookie";
import { NONE_GRANTED, parseConsent, type Consent } from "@/app/lib/consent/categories";

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
