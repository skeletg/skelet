import { headers } from "next/headers";
import { channel } from "next/root-params";
import { getLocale } from "next-intl/server";

import { CHANNEL_HEADER, PINNED_CHANNEL_ID } from "@/app/lib/channel";
import { getChannelLocales, type ChannelLocales } from "@/app/lib/locale/backend-locales";

// From the [locale] route segment, never from a cookie. A cookie can't be read on the routes that
// serve most of the site (they render without a request), so a cookie-driven language silently
// served everyone the default one - the segment is part of the cache key instead, so each language
// gets its own cached page
export async function getCurrentLocale(): Promise<string> {
  return getLocale();
}

function rethrowIfDynamicBailout(err: unknown): void {
  const digest = (err as { digest?: unknown } | null | undefined)?.digest;
  if (typeof digest === "string" && digest.startsWith("DYNAMIC_SERVER_USAGE")) throw err;
}

// Pages read the [channel] segment, so static renders are per channel; route handlers and actions read the proxy's header
export async function getCurrentChannelId(): Promise<string> {
  try {
    const c = await channel();
    if (c) return c;
  } catch {
    // not under the [channel] root layout
  }
  try {
    const c = (await headers()).get(CHANNEL_HEADER);
    if (c) return c;
  } catch (err) {
    rethrowIfDynamicBailout(err);
  }
  return PINNED_CHANNEL_ID;
}

export async function getCurrentChannelLocales(): Promise<ChannelLocales> {
  return getChannelLocales(await getCurrentChannelId());
}
