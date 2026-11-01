import { gql } from "./gql";
import { siteOrigin } from "./seo";
import { CurrentChannelDocument, type CurrentChannelQuery } from "../graphql/channel.gql";

export type Channel = NonNullable<CurrentChannelQuery["currentChannel"]>;

// Read on every page including the uncached cart, so it's held here - /api/revalidate drops it on config:public
let held: { channel: Channel; at: number } | null = null;
const TTL_MS = 5 * 60_000;

export async function getChannel(): Promise<Channel> {
  if (held && Date.now() - held.at < TTL_MS) return held.channel;
  const { currentChannel } = await gql(CurrentChannelDocument);
  if (!currentChannel) throw new Error("STOREFRONT_CHANNEL_ID names no active channel");
  held = { channel: currentChannel, at: Date.now() };
  return currentChannel;
}

// The channel plus the canonical origin every absolute URL is built on
export async function getSite(requestOrigin: string): Promise<{ channel: Channel; origin: string }> {
  const channel = await getChannel();
  return { channel, origin: siteOrigin(channel.publicUrl, requestOrigin) };
}

export function forgetChannel(): void {
  held = null;
}
