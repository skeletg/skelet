// The one place a template reports a shop moment: GA4's dataLayer. The tracking and Signals readers read it, and so
// does any tag a template declares, so one push feeds all of them. Pushing with no reader installed is harmless - the
// array just fills. Never push an email, a phone number or a customer id: every tag on the page can read it

import type { Ecommerce, EventName } from "./contract.gen";

declare global {
  interface Window {
    dataLayer?: unknown[];
  }
}

// What travels with an event besides its name and id - GA4's ecommerce block and the event's own parameters
export type EventData = { ecommerce?: Ecommerce; search_term?: string };

// pushEvent adds one event in GA4's shape and returns its id, which the readers forward so a destination can dedupe a
// copy it also gets from the server. The previous event's ecommerce is cleared first, as GA4 asks
export function pushEvent(name: EventName, data: EventData = {}, id: string = crypto.randomUUID()): string {
  if (typeof window === "undefined") return id;
  const layer = (window.dataLayer ??= []);
  if (data.ecommerce) layer.push({ ecommerce: null });
  layer.push({ event: name, event_id: id, ...data });
  return id;
}

// eventAttrs is the same event as attributes on server-rendered markup, so a Next server component or an Astro page
// reports a view with no client component of its own - the tracking reader pushes each marked element once
export function eventAttrs(name: EventName, data: EventData = {}): { "data-track": EventName; "data-track-data": string } {
  return { "data-track": name, "data-track-data": JSON.stringify(data) };
}
