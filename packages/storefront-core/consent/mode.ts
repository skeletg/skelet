import { print } from "graphql";

import { FRONT_API_URL } from "../backend-url";
import { PublicConfigDocument, type PublicConfigQuery } from "../config/operations.gql";
import type { ConsentMode } from "./categories";

// The store's cookie consent mode and whether it counts visitors who decline analytics. A plain fetch with a module-level TTL cache, like backend-locales, because the proxy
// reads it on every request and has no render scope for gqlFetch or react cache(). One request a minute per channel

const QUERY = print(PublicConfigDocument);
const TTL_MS = 60_000;
const PATH = "general/privacy/cookie_consent";
const COUNTS = "general/privacy/aggregate_counts";

// counts says whether to report page views from visitors who decline analytics - the backend drops them anyway when it's off,
// so an unread setting leaves it on rather than lose views
export type Privacy = { mode: ConsentMode; counts: boolean };
const ASK_FIRST: Privacy = { mode: "opt_in", counts: true };

const cache = new Map<string, { at: number; value: Privacy }>();

// Anything unset, unknown or unreachable reads as opt_in - asking is the side that can't break the law
export async function getPrivacy(channelId: string): Promise<Privacy> {
  if (!channelId) return ASK_FIRST;
  const cached = cache.get(channelId);
  if (cached && Date.now() - cached.at < TTL_MS) return cached.value;
  try {
    const r = await fetch(FRONT_API_URL, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Channel-Id": channelId },
      body: JSON.stringify({ query: QUERY }),
      signal: AbortSignal.timeout(2000),
      cache: "no-store",
    });
    if (!r.ok) throw new Error(String(r.status));
    const body = (await r.json()) as { data?: PublicConfigQuery };
    const config = body.data?.publicConfig ?? [];
    const value: Privacy = {
      mode: config.find((c) => c.path === PATH)?.value === "opt_out" ? "opt_out" : "opt_in",
      counts: config.find((c) => c.path === COUNTS)?.value !== "false",
    };
    cache.set(channelId, { at: Date.now(), value });
    return value;
  } catch {
    // Keep the last good answer - flipping an opt-out shop to opt-in would delete every visitor's store-set consent
    return cached?.value ?? ASK_FIRST;
  }
}

export async function getConsentMode(channelId: string): Promise<ConsentMode> {
  return (await getPrivacy(channelId)).mode;
}
