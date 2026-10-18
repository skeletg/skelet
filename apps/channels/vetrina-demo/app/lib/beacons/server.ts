import "server-only";

import { getAccessToken } from "@/app/lib/auth/session";
import { BACKEND_INTERNAL_URL } from "@/app/lib/backend-url";
import { getCurrentChannelId } from "@/app/lib/locale/read";
import { shopperHeaders } from "@/app/lib/shopper-headers";
import type { BeaconPath } from "./paths";

// A server action's POST to a /t/ endpoint, sent on the shopper's behalf: their IP, anon id and channel ride along,
// and the session goes as a bearer token (as gqlFetch sends it), so a signed-in shopper is credited to their account
export async function postBeacon(path: BeaconPath, body: unknown): Promise<Response> {
  const [channelId, accessToken, shopper] = await Promise.all([getCurrentChannelId(), getAccessToken(), shopperHeaders()]);
  const headers: Record<string, string> = {
    ...shopper,
    "Content-Type": "application/json",
    "X-Channel-Id": channelId || "unresolved",
  };
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`;
  return fetch(`${BACKEND_INTERNAL_URL}${path}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
    cache: "no-store",
    signal: AbortSignal.timeout(5000),
  });
}
