// Server-side fetcher for the resolved channel (name, currency, logo, support email,
// etc.)

import { cache } from "react";

import { gqlFetch } from "@/app/lib/graphql";
import { getStoreNameSetting, surfaceCacheOpts, NAV_SURFACE, NAV_TAG } from "@/app/lib/config/read";
import { CurrentChannelDocument } from "@/app/lib/channel/operations.gql";
import type { Channel } from "@/app/lib/channel/types";

export const getChannel = cache(async (): Promise<Channel | null> => {
  try {
    const cacheOpts = await surfaceCacheOpts(NAV_SURFACE, { tags: [NAV_TAG] });
    const res = await gqlFetch(CurrentChannelDocument, {}, cacheOpts);
    return res.data.currentChannel;
  } catch {
    // Treat read failures as "no store" - the rest of the page can still render (Header
    // degrades to its hardcoded brand), and the next request will retry
    return null;
  }
});

// The name to show shoppers, resolved the same way the backend resolves it for transactional email
// (internal/flow.customerFacingStoreName): the branding setting when it is set, else the channel's
// own name. Header, footer, homepage and the Organization/WebSite JSON-LD all go through this, so a
// store that sets it is renamed everywhere at once
export const getStoreName = cache(async (): Promise<string> => {
  const [configured, channel] = await Promise.all([getStoreNameSetting(), getChannel()]);
  return configured ?? channel?.name ?? "Skelet";
});
