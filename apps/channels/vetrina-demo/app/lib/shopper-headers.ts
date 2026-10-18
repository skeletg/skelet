import "server-only";

import { cookies, headers as nextHeaders } from "next/headers";

import { AID_COOKIE } from "@/app/lib/analytics/visitor";
import { clientIP } from "@/app/lib/client-ip";
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
    // The SEO crawler's bypass token, so a crawl of a few thousand pages is not refused by the per-IP limiter
    const crawl = inbound.get("x-skelet-crawl");
    if (crawl) out["X-Skelet-Crawl"] = crawl;
  } catch (err) {
    rethrowIfDynamicBailout(err);
    // not in a request scope - nothing to forward
  }
  // The visitor's anon id (the Lens cookie), so guest commerce attributes to the same visitor as their browsing and
  // stitches to a customer once they sign in
  try {
    const aid = (await cookies()).get(AID_COOKIE)?.value;
    if (aid) out["X-Anon-Id"] = aid;
  } catch (err) {
    rethrowIfDynamicBailout(err);
  }
  return out;
}
