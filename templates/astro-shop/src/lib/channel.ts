import { getStoreName, loadChannel } from "@skelet/storefront-core/channel/read";
import type { Channel as CoreChannel } from "@skelet/storefront-core/channel/types";

import type { Currency } from "./format";
import { siteOrigin } from "./seo";

export type Channel = Pick<CoreChannel, "name" | "defaultLocale" | "publicUrl"> & { currency: Currency };

// Read on every page including the uncached cart - the core keeps it for the request and its data cache for longer,
// until /api/revalidate drops nav:all
export async function getChannel(): Promise<Channel> {
  const [channel, name] = await Promise.all([loadChannel(), getStoreName()]);
  if (!channel) {
    throw new Error("This host serves no active channel - check STOREFRONT_CHANNEL_ID (or STOREFRONT_CHANNEL_HOSTS) against admin > channels");
  }
  return { ...channel, name, currency: { ...channel.currency, locale: channel.defaultLocale } };
}

// The channel plus the canonical origin every absolute URL is built on
export async function getSite(requestOrigin: string): Promise<{ channel: Channel; origin: string }> {
  const channel = await getChannel();
  return { channel, origin: siteOrigin(channel.publicUrl, requestOrigin) };
}
