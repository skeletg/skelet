// Server-side fetcher for the resolved channel (name, currency, logo, support email,
// etc.)

import { perRequest, rethrowControlFlow } from "../request";
import { gqlFetch } from "../graphql";
import { getStoreNameSetting, surfaceCacheOpts, NAV_SURFACE, NAV_TAG } from "../config/read";
import { CurrentChannelDocument } from "./operations.gql";
import type { Channel } from "./types";

export const loadChannel = perRequest(async (): Promise<Channel | null> => {
  const cacheOpts = await surfaceCacheOpts(NAV_SURFACE, { tags: [NAV_TAG] });
  return (await gqlFetch(CurrentChannelDocument, {}, cacheOpts)).data.currentChannel;
});

export const getChannel = perRequest(async (): Promise<Channel | null> => {
  try {
    return await loadChannel();
  } catch (err) {
    rethrowControlFlow(err);
    // Treat read failures as "no store" - the rest of the page can still render (Header
    // degrades to its hardcoded brand), and the next request will retry
    return null;
  }
});

// The name to show shoppers, resolved the same way the backend resolves it for transactional email
// (internal/flow.customerFacingStoreName): the branding setting when it is set, else the channel's
// own name. Header, footer, homepage and the Organization/WebSite JSON-LD all go through this, so a
// store that sets it is renamed everywhere at once
export const getStoreName = perRequest(async (): Promise<string> => {
  const [configured, channel] = await Promise.all([getStoreNameSetting(), getChannel()]);
  return configured ?? channel?.name ?? "Skelet";
});
