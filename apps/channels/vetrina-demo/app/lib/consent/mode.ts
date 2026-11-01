import { print } from "graphql";

import { BACKEND_INTERNAL_URL } from "@/app/lib/backend-url";
import { PublicConfigDocument, type PublicConfigQuery } from "@/app/lib/config/operations.gql";
import type { ConsentMode } from "@/app/lib/consent/categories";

// The store's cookie consent mode and which affiliate cookies it made necessary. A plain fetch with a module-level TTL cache, like backend-locales, because the proxy
// reads it on every request and has no render scope for gqlFetch or react cache(). One request a minute per channel

const FRONT_API_PATH = process.env.FRONT_API_SLUG
  ? `/api/front/${process.env.FRONT_API_SLUG}/graphql`
  : "/api/front/graphql";
const QUERY = print(PublicConfigDocument);
const TTL_MS = 60_000;
const PATH = "general/privacy/cookie_consent";
// The affiliate networks whose click cookie the merchant made necessary, by the click-id key each one stores
const NECESSARY: Record<string, string> = {
  "general/privacy/cj_cookie_necessary": "cjevent",
  "general/privacy/awin_cookie_necessary": "awc",
};

export type Privacy = { mode: ConsentMode; necessary: Set<string> };
const ASK_FIRST: Privacy = { mode: "opt_in", necessary: new Set() };

const cache = new Map<string, { at: number; value: Privacy }>();

// Anything unset, unknown or unreachable reads as opt_in with nothing necessary - asking is the side that can't break the law
export async function getPrivacy(channelId: string): Promise<Privacy> {
  if (!channelId) return ASK_FIRST;
  const cached = cache.get(channelId);
  if (cached && Date.now() - cached.at < TTL_MS) return cached.value;
  try {
    const r = await fetch(`${BACKEND_INTERNAL_URL}${FRONT_API_PATH}`, {
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
      necessary: new Set(config.filter((c) => NECESSARY[c.path] && c.value === "true").map((c) => NECESSARY[c.path])),
    };
    cache.set(channelId, { at: Date.now(), value });
    return value;
  } catch {
    return ASK_FIRST;
  }
}

export async function getConsentMode(channelId: string): Promise<ConsentMode> {
  return (await getPrivacy(channelId)).mode;
}
