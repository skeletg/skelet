import "server-only";

import { cookies, headers as nextHeaders } from "next/headers";

import { SESSION_COOKIE, validSession } from "@/app/lib/analytics/session";
import { AID_COOKIE } from "@/app/lib/analytics/visitor";
import { clientIP } from "@/app/lib/client-ip";
import { CONSENT_COOKIE } from "@/app/lib/consent/cookie";
import { NONE_GRANTED, parseConsent, serializeConsent } from "@/app/lib/consent/categories";
import { clickIdsHeader } from "@/app/lib/consent/click-ids";
import { rethrowIfDynamicBailout } from "@/app/lib/dynamic-bailout";

// Who a server-side call to the backend is made for. Without these the backend sees this Next server instead of the
// shopper: every visitor shares one per-IP rate-limit bucket (brute-force pooling, cross-user lockout), and guest
// activity loses the anonymous visitor it belongs to. Every server→backend request sends them - gqlFetch, the
// experiments feed and postBeacon - so none of them can drift
export async function shopperHeaders(): Promise<Record<string, string>> {
  const out: Record<string, string> = {};
  try {
    const inbound = await nextHeaders();
    const ip = clientIP(inbound);
    if (ip) out["X-Forwarded-For"] = ip;
    // The shopper's browser rather than this server's fetch - session lists, bot checks and ad platforms' match keys read it
    const ua = inbound.get("user-agent");
    if (ua) out["User-Agent"] = ua;
    // The SEO crawler's bypass token, so a crawl of a few thousand pages is not refused by the per-IP limiter
    const crawl = inbound.get("x-skelet-crawl");
    if (crawl) out["X-Skelet-Crawl"] = crawl;
  } catch (err) {
    rethrowIfDynamicBailout(err);
    // not in a request scope - nothing to forward
  }
  // The visitor's cookie consent, so the backend forwards an event only where it was allowed - and their anon id (the
  // Lens cookie), so guest commerce attributes to the same visitor as their browsing and stitches to a customer once
  // they sign in. The id goes only with analytics consent: on the request that withdraws it the cookie is still here
  try {
    const store = await cookies();
    const consent = parseConsent(store.get(CONSENT_COOKIE)?.value);
    // Always stated: no choice yet grants nothing, and a missing header then only ever means a caller that isn't a shop
    out["X-Consent"] = serializeConsent(consent ?? NONE_GRANTED);
    const aid = store.get(AID_COOKIE)?.value;
    if (aid && consent?.analytics) out["X-Anon-Id"] = aid;
    const sid = store.get(SESSION_COOKIE)?.value;
    if (consent?.analytics && validSession(sid)) out["X-Session-Id"] = sid;
    // Click ids for a purchase sent server to server - see clickIdsHeader for which go without ads consent
    const clicks = clickIdsHeader((name) => store.get(name)?.value, consent?.ads ?? false);
    if (clicks) out["X-Click-Ids"] = clicks;
  } catch (err) {
    rethrowIfDynamicBailout(err);
  }
  return out;
}
