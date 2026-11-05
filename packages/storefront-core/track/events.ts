// Browser events for the shop's server-side destinations. They go to /t/e on the shop's own origin, whose route adds
// the visitor's consent and click ids from cookies and forwards to the backend - no third-party script, nothing an ad
// blocker sees as tracking, and the backend still sends each one only where the visitor's consent allows. Cart changes,
// purchases and sign-ups are never sent from here: the server reports those itself

export const TRACK_PATH = "/t/e";

// Mirrors the backend's tracking.BrowserEvent
const BROWSER_EVENTS = new Set([
  "page_view",
  "view_item_list",
  "select_item",
  "view_item",
  "view_cart",
  "begin_checkout",
  "add_shipping_info",
  "add_payment_info",
]);

export function isBrowserEvent(name: string): boolean {
  return BROWSER_EVENTS.has(name);
}

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

export function trackPayload(name: string, eventId: string, ecommerce: unknown, href: string, referrer: string, locale: string) {
  return { event_id: eventId, name, ecommerce: ecommerce ?? undefined, page_url: cleanUrl(href), referrer: cleanUrl(referrer), locale };
}

export function track(name: string, eventId: string, ecommerce?: unknown) {
  if (!isBrowserEvent(name) || typeof navigator === "undefined") return;
  const body = JSON.stringify(trackPayload(name, eventId, ecommerce, location.href, document.referrer, document.documentElement.lang));
  // sendBeacon survives the page being left, which a view or checkout step often is right away
  if (!navigator.sendBeacon?.(TRACK_PATH, new Blob([body], { type: "application/json" }))) {
    void fetch(TRACK_PATH, { method: "POST", body, headers: { "Content-Type": "application/json" }, keepalive: true }).catch(() => {});
  }
}
