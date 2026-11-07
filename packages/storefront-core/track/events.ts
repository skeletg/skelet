// Browser events for the shop's server-side destinations. They go to /t/e on the shop's own origin, whose route adds
// the visitor's consent and click ids from cookies and forwards to the backend - no third-party script, nothing an ad
// blocker sees as tracking, and the backend still sends each one only where the visitor's consent allows. Cart changes,
// purchases and sign-ups the server reports itself, so the backend takes only the events a browser may report and drops
// any other name sent here - one list, the backend's tracking.BrowserEvent

export const TRACK_PATH = "/t/e";

// The page without its query string, which can carry a guest's email or a token - campaign tags are what reporting needs
export function cleanUrl(href: string): string {
  try {
    const url = new URL(href);
    const kept = new URLSearchParams();
    for (const [k, v] of url.searchParams) if (k.startsWith("utm_")) kept.set(k, v);
    url.search = kept.toString();
    url.hash = "";
    return url.toString();
  } catch {
    return "";
  }
}

// What an event carries besides its name: GA4's ecommerce block, and the term of a search
export type TrackData = { ecommerce?: unknown; searchTerm?: string };

// The page an event was sent from, as the browser has it
export type TrackPage = { href: string; referrer: string; locale: string; title?: string };

export function trackPayload(name: string, eventId: string, data: TrackData, page: TrackPage) {
  return {
    event_id: eventId,
    name,
    ecommerce: data.ecommerce ?? undefined,
    page_url: cleanUrl(page.href),
    referrer: cleanUrl(page.referrer),
    locale: page.locale,
    page_title: page.title?.slice(0, 300) || undefined,
    search_term: data.searchTerm?.trim().slice(0, 200) || undefined, // the term a search carries, which cleanUrl drops with the query
  };
}

export function track(name: string, eventId: string, data: TrackData = {}) {
  if (typeof navigator === "undefined") return;
  const page = { href: location.href, referrer: document.referrer, locale: document.documentElement.lang, title: document.title };
  const body = JSON.stringify(trackPayload(name, eventId, data, page));
  // sendBeacon survives the page being left, which a view or checkout step often is right away
  if (!navigator.sendBeacon?.(TRACK_PATH, new Blob([body], { type: "application/json" }))) {
    void fetch(TRACK_PATH, { method: "POST", body, headers: { "Content-Type": "application/json" }, keepalive: true }).catch(() => {});
  }
}
