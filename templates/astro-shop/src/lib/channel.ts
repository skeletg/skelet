import { getChannel as channelOf } from "@skelet/storefront-core/channel/read";
import type { Channel as CoreChannel } from "@skelet/storefront-core/channel/types";

import { siteOrigin } from "./seo";

// What the pages read of it
export type Channel = Pick<CoreChannel, "name" | "defaultLocale" | "publicUrl" | "currency">;

// Read on every page including the uncached cart - the core keeps it for the request and its data cache for longer,
// until /api/revalidate drops config:public
export async function getChannel(): Promise<Channel> {
  const channel = await channelOf();
  if (!channel) throw new Error("STOREFRONT_CHANNEL_ID names no active channel");
  return channel;
}

// The channel plus the canonical origin every absolute URL is built on
export async function getSite(requestOrigin: string): Promise<{ channel: Channel; origin: string }> {
  const channel = await getChannel();
  return { channel, origin: siteOrigin(channel.publicUrl, requestOrigin) };
}
