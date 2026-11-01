"use client";

import { useEffect } from "react";

import { syncGtm } from "@/app/lib/analytics/gtm";
import { parseConsent } from "@/app/lib/consent/categories";
import { useConsentChoice } from "@/app/lib/consent/use-consent";

// Renders nothing - boots the shop's web GTM container once the visitor allows a category, and passes every later
// change of mind to it. Without a container id it isn't mounted at all
export function GtmLoader({ containerId, origin }: { containerId: string; origin: string }) {
  const choice = useConsentChoice();
  useEffect(() => {
    const consent = parseConsent(choice);
    if (consent) syncGtm(window, document, containerId, origin, consent);
  }, [choice, containerId, origin]);
  return null;
}
