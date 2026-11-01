"use client";

import { useEffect } from "react";

import { readClientConsent } from "@/app/lib/consent/client";
import { useAnalyticsConsent } from "@/app/lib/consent/use-consent";
import { track } from "@/app/lib/track";

type DataLayerEntry = Record<string, unknown>;

declare global {
  interface Window {
    dataLayer?: DataLayerEntry[];
  }
}

export function AnalyticsEvent({
  enabled,
  event,
  eventId,
  payload,
  stripParam,
}: {
  enabled?: boolean; // omit on cached routes - a server-side consent read forces them dynamic
  event: string;
  // Shared with the server's own copy so a platform counts it once - a purchase's is the order id. Omitted, each firing mints one
  eventId?: string;
  payload?: Record<string, unknown>;
  // The query param that triggered this one-shot event (?welcome=1, ?just_placed=1) - removed from the address bar once
  // fired, so a refresh, bookmark or shared link can't replay a sign-up or a purchase into reporting
  stripParam?: string;
}) {
  const clientConsent = useAnalyticsConsent();
  const on = enabled ?? clientConsent;
  useEffect(() => {
    if (typeof window === "undefined") return;
    const id = eventId ?? crypto.randomUUID();
    if (on) {
      window.dataLayer ??= [];
      if (payload?.ecommerce) window.dataLayer.push({ ecommerce: null }); // GA4's reset, so the last event's items don't merge in
      window.dataLayer.push({ event, event_id: id, ...(payload ?? {}) });
    }
    // The server-side destinations each need their own category, which the backend checks - so any grant sends it
    const c = readClientConsent();
    if (c.analytics || c.ads || c.personalization) track(event, id, payload?.ecommerce);
    // Intentionally do not depend on `payload` - the consumer renders this component as
    // part of a one-shot view, and React.StrictMode would otherwise double-fire on
    // every re-render that produces a new object identity
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [on, event]);

  // Separate from the push: the param must go even when consent says don't track, or a
  // later consent grant would fire it after all
  useEffect(() => {
    if (!stripParam || typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (!url.searchParams.has(stripParam)) return;
    url.searchParams.delete(stripParam);
    window.history.replaceState(null, "", url.pathname + url.search + url.hash);
  }, [stripParam]);

  return null;
}
