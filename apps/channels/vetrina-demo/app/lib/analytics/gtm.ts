// The optional web Google Tag Manager container. Loaded only once the visitor allows a cookie category - Consent Mode's
// "basic" setup, since the "advanced" one pings Google before consent - and only after the page has loaded and gone
// idle, so it never competes with the shop. Without a container configured nothing here runs at all: events already
// reach the platforms server to server

import type { Consent } from "@/app/lib/consent/categories";

type DataLayerWindow = Window & { dataLayer?: unknown[]; skeletGtm?: boolean };

const CONTAINER = /^GTM-[A-Z0-9]{4,12}$/;
const GOOGLE = "https://www.googletagmanager.com";

export function validContainer(id: string): boolean {
  return CONTAINER.test(id);
}

// Where gtm.js comes from: Google, or a server-side GTM container serving it first-party. Only an https origin
export function scriptURL(id: string, origin: string): string {
  let base = GOOGLE;
  try {
    const u = new URL(origin);
    if (u.protocol === "https:") base = u.origin;
  } catch {
    // empty or malformed - Google's own
  }
  return `${base}/gtm.js?id=${encodeURIComponent(id)}`;
}

// Consent Mode v2's signals from the banner's categories
export function consentSignals(c: Consent) {
  const v = (b: boolean) => (b ? "granted" : "denied");
  return {
    analytics_storage: v(c.analytics),
    ad_storage: v(c.ads),
    ad_user_data: v(c.ads),
    ad_personalization: v(c.personalization),
  };
}

// The Arguments object of a call - gtag pushes that itself, and GTM reads consent commands only in that shape
function argumentsOf(...values: unknown[]): IArguments {
  void values;
  // eslint-disable-next-line prefer-rest-params
  return arguments;
}

function gtag(w: DataLayerWindow, ...args: unknown[]) {
  w.dataLayer ??= [];
  w.dataLayer.push(argumentsOf(...args));
}

// Boots the container once, or passes a changed choice to one already running. Returns whether GTM is on the page
export function syncGtm(w: DataLayerWindow, doc: Document, id: string, origin: string, c: Consent): boolean {
  const granted = c.analytics || c.ads || c.personalization;
  if (w.skeletGtm) {
    gtag(w, "consent", "update", consentSignals(c)); // a withdrawal reaches a container that can't be unloaded
    return true;
  }
  if (!granted || !validContainer(id)) return false;
  w.skeletGtm = true;
  gtag(w, "consent", "default", { ...consentSignals({ analytics: false, ads: false, personalization: false }), wait_for_update: 500 });
  gtag(w, "consent", "update", consentSignals(c));
  w.dataLayer!.push({ "gtm.start": Date.now(), event: "gtm.js" });
  const load = () => {
    const s = doc.createElement("script");
    s.async = true;
    s.src = scriptURL(id, origin);
    doc.head.appendChild(s);
  };
  const idle = () => ("requestIdleCallback" in w ? w.requestIdleCallback(load, { timeout: 3000 }) : setTimeout(load, 1));
  if (doc.readyState === "complete") idle();
  else w.addEventListener("load", idle, { once: true });
  return true;
}
