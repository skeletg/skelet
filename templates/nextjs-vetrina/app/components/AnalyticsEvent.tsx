"use client";

import { useEffect } from "react";

import { track, type TrackData } from "@skelet/storefront-core/track/events";
import { trackingAllowed } from "@skelet/storefront-react/consent/client";

export function AnalyticsEvent({
  event,
  payload,
}: {
  event: string;
  payload?: TrackData;
}) {
  useEffect(() => {
    if (typeof window === "undefined") return;
    if (trackingAllowed()) track(event, crypto.randomUUID(), payload);
    // Intentionally do not depend on `payload` - the consumer renders this component as
    // part of a one-shot view, and React.StrictMode would otherwise double-fire on
    // every re-render that produces a new object identity
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [event]);

  return null;
}
