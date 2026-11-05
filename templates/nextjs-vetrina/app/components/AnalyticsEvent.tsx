"use client";

import { useEffect } from "react";

import { track } from "@skelet/storefront-core/track/events";
import { readClientConsent } from "@skelet/storefront-react/consent/client";

export function AnalyticsEvent({
  event,
  eventId,
  payload,
  stripParam,
}: {
  event: string;
  // Shared with the server's own copy so a platform counts it once - a purchase's is the order id. Omitted, each firing mints one
  eventId?: string;
  payload?: Record<string, unknown>;
  // The query param that triggered this one-shot event (?welcome=1, ?just_placed=1) - removed from the address bar once
  // fired, so a refresh, bookmark or shared link can't replay a sign-up or a purchase into reporting
  stripParam?: string;
}) {
  useEffect(() => {
    if (typeof window === "undefined") return;
    const id = eventId ?? crypto.randomUUID();
    // The server-side destinations each need their own category, which the backend checks - so any grant sends it
    const c = readClientConsent();
    if (c.analytics || c.ads || c.personalization) track(event, id, payload?.ecommerce, payload?.search_term as string | undefined);
    // Intentionally do not depend on `payload` - the consumer renders this component as
    // part of a one-shot view, and React.StrictMode would otherwise double-fire on
    // every re-render that produces a new object identity
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [event]);

  // Separate from the send: the param must go even when consent says don't track, or a
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
