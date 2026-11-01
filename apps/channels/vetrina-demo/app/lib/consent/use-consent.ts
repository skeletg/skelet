"use client";

import { useSyncExternalStore } from "react";

import { serializeConsent } from "@/app/lib/consent/categories";
import { readClientConsent } from "@/app/lib/consent/client";

const noopSubscribe = () => () => {}; // the cookie never pushes - useSyncExternalStore is still right, it reads a browser value without a setState-in-effect cascade

export function useAnalyticsConsent(): boolean {
  return useSyncExternalStore(
    noopSubscribe,
    () => readClientConsent().analytics,
    () => false, // consent-off is the correct GDPR default before the cookie is known
  );
}

export function useConsentDecided(): boolean {
  return useSyncExternalStore(
    noopSubscribe,
    () => readClientConsent().decided,
    () => true, // assume decided on the server so the banner never flashes at someone who chose
  );
}

// The whole choice, as its cookie string - a string snapshot stays equal between reads, which an object would not
export function useConsentChoice(): string {
  return useSyncExternalStore(
    noopSubscribe,
    () => serializeConsent(readClientConsent()),
    () => "v2:a0.d0.p0", // nothing granted until the cookie is known
  );
}
