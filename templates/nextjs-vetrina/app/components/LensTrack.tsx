"use client";

import { useEffect } from "react";

import { useMarketingConsent } from "@skelet/storefront-react/consent/use-consent";

// Reads consent itself rather than taking it as a prop: the listings that render this live under the
// force-static catch-all, where a server-side cookie read always came back "no consent" and silently
// dropped the event for visitors who had accepted
export function LensTrack({
  event,
  props,
}: {
  event: string;
  props?: Record<string, unknown>;
}) {
  const enabled = useMarketingConsent();
  useEffect(() => {
    if (!enabled || !event || typeof window === "undefined") return;
    (window.__lensQueue ??= []).push({ name: event, props });
    // Drain now if the tracker is already active; otherwise it drains on activation
    window.lens?.flushQueue?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, event]);

  return null;
}
