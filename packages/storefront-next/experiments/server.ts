import "server-only";

import { cookies } from "next/headers";

import { getAccessToken } from "../auth/session";
import { getChannel } from "../channel/read";
import { BACKEND_INTERNAL_URL } from "../backend-url";
import { AID_COOKIE } from "../analytics/visitor";
import { assign, type Experiment } from "./assign";
import { BEACON } from "../beacons/paths";
import { shopperHeaders } from "../shopper-headers";

// Server-side Prism assignment

// Hits the backend config endpoint directly (not via the /t rewrite) with the channel
// id the proxy can't inject
async function loadExperiments(channelId: string, accessToken?: string | null): Promise<Experiment[]> {
  if (!channelId) return [];
  const headers: Record<string, string> = { "X-Channel-Id": channelId, Accept: "application/json" };
  // Forward the customer's session JWT so the backend can serve segment-targeted experiments the VERIFIED customer is
  // eligible for (anonymous → none). The backend does not honor a bare customer-id header, which would be forgeable
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`;
  // Without the shopper's IP every SSR render shares one rate-limit bucket and experiments silently disappear under load
  Object.assign(headers, await shopperHeaders());
  try {
    const r = await fetch(`${BACKEND_INTERNAL_URL}${BEACON.experiments}`, { headers, cache: "no-store" });
    return r.ok ? ((await r.json()) as Experiment[]) : [];
  } catch {
    return [];
  }
}

export type Assignment = { variation: string; value: string; preview: boolean };
export async function assignExperiment(key: string, force?: string): Promise<Assignment | null> {
  const [channel, accessToken] = await Promise.all([getChannel(), getAccessToken()]);
  const exps = await loadExperiments(channel?.id ?? "", accessToken);
  const exp = exps.find((e) => e.key === key);
  if (!exp) return null;

  if (force) {
    const fv = exp.variations.find((v) => v.key === force);
    if (fv) return { variation: fv.key, value: fv.value, preview: true };
  }

  const store = await cookies();
  const anon = store.get(AID_COOKIE)?.value ?? "";
  if (!anon) return null;
  const v = assign(exp, anon);
  if (!v) return null;
  return { variation: v.key, value: v.value, preview: false };
}
export function forcedFor(key: string, raw?: string | string[]): string | undefined {
  const s = Array.isArray(raw) ? raw[0] : raw;
  if (!s || s === "off" || s === "clear") return undefined;
  for (const pair of s.split(",")) {
    const [k, v] = pair.split(":");
    if (k?.trim() === key && v) return v.trim();
  }
  return undefined;
}
