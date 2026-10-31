"use client";

import { CONSENT_COOKIE } from "@/app/lib/consent/cookie";

export type ClientConsent = { choice: "all" | "essential" | null; decided: boolean; marketing: boolean };

// httpOnly is false on purpose, so a cached page can resolve consent without a server render
export function readClientConsent(): ClientConsent {
  if (typeof document === "undefined") return { choice: null, decided: false, marketing: false };
  const hit = document.cookie
    .split("; ")
    .find((row) => row.startsWith(`${CONSENT_COOKIE}=`));
  const raw = hit?.split("=")[1];
  const choice = raw === "all" || raw === "essential" ? raw : null;
  return { choice, decided: choice !== null, marketing: choice === "all" };
}
