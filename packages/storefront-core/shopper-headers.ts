import { request, rethrowControlFlow } from "./request";
import { AID_COOKIE } from "./analytics/visitor";
import { clientIP } from "./client-ip";
import { CONSENT_COOKIE } from "./consent/cookie";
import { NONE_GRANTED, parseConsent, serializeConsent } from "./consent/categories";
import { clickIdsHeader } from "./consent/click-ids";

// Who a server-side call to the backend is made for. Without these the backend sees this Next server instead of the
// shopper: every visitor shares one per-IP rate-limit bucket (brute-force pooling, cross-user lockout), and guest
// activity loses the anonymous visitor it belongs to. Every server→backend request sends them - gqlFetch, the
// experiments feed and postBeacon - so none of them can drift
export async function shopperHeaders(): Promise<Record<string, string>> {
  const out: Record<string, string> = {};
  try {
    const inbound = await request().headers();
    const ip = clientIP(inbound);
    if (ip) out["X-Forwarded-For"] = ip;
    // The shopper's browser rather than this server's fetch - session lists, bot checks and ad platforms' match keys read it
    const ua = inbound.get("user-agent");
    if (ua) out["User-Agent"] = ua;
    // The SEO crawler's bypass token, so a crawl of a few thousand pages is not refused by the per-IP limiter
    const crawl = inbound.get("x-skelet-crawl");
    if (crawl) out["X-Skelet-Crawl"] = crawl;
  } catch (err) {
    rethrowControlFlow(err);
    // not in a request scope - nothing to forward
  }
  // The visitor's anon id (the Lens cookie), so guest commerce attributes to the same visitor as their browsing and
  // stitches to a customer once they sign in. Only with analytics consent: on the request that withdraws it the cookie is still here
  try {
    const store = await request().cookies();
    const consent = parseConsent(store.get(CONSENT_COOKIE)?.value);
    // Always stated: no choice yet grants nothing, and a missing header then only ever means a caller that isn't a shop
    out["X-Consent"] = serializeConsent(consent ?? NONE_GRANTED);
    const aid = store.get(AID_COOKIE)?.value;
    if (aid && consent?.analytics) out["X-Anon-Id"] = aid;
    // Click ids for a purchase sent server to server - see clickIdsHeader for which go without ads consent
    const clicks = clickIdsHeader((name) => store.get(name)?.value, consent?.ads ?? false);
    if (clicks) out["X-Click-Ids"] = clicks;
  } catch (err) {
    rethrowControlFlow(err);
  }
  return out;
}
