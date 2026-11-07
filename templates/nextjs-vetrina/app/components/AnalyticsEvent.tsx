"use client";

import { useEffect } from "react";

import { track } from "@skelet/storefront-core/track/events";
import { readClientConsent } from "@skelet/storefront-react/consent/client";

export function AnalyticsEvent({
  event,
  payload,
}: {
  event: string;
  payload?: Record<string, unknown>;
}) {
  useEffect(() => {
    if (typeof window === "undefined") return;
    const id = crypto.randomUUID();
    // The server-side destinations each need their own category, which the backend checks - so any grant sends it
    const c = readClientConsent();
    if (c.analytics || c.ads || c.personalization) track(event, id, payload?.ecommerce, payload?.search_term as string | undefined);
    // Intentionally do not depend on `payload` - the consumer renders this component as
    // part of a one-shot view, and React.StrictMode would otherwise double-fire on
    // every re-render that produces a new object identity
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [event]);

  return null;
}
