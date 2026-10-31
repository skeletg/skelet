"use client";

import { useEffect } from "react";

import { useAnalyticsConsent } from "@/app/lib/consent/use-consent";

type DataLayerEntry = Record<string, unknown> & { event: string };

declare global {
  interface Window {
    dataLayer?: DataLayerEntry[];
  }
}

export function AnalyticsEvent({
  enabled,
  event,
  payload,
  stripParam,
}: {
  enabled?: boolean; // omit on cached routes - a server-side consent read forces them dynamic
  event: string;
  payload?: Record<string, unknown>;
  // The query param that triggered this one-shot event (?welcome=1, ?just_placed=1) - removed from the address bar once
  // fired, so a refresh, bookmark or shared link can't replay a sign-up or a purchase into reporting
  stripParam?: string;
}) {
  const clientConsent = useAnalyticsConsent();
  const on = enabled ?? clientConsent;
  useEffect(() => {
    if (!on) return;
    if (typeof window === "undefined") return;
    window.dataLayer ??= [];
    window.dataLayer.push({ event, ...(payload ?? {}) });
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
