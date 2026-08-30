// Storefront locale state

import { getLocale } from "next-intl/server";
import { requireChannelId } from "@/app/lib/channel";

// A storefront serves exactly ONE channel, pinned per deployment via STOREFRONT_CHANNEL_ID, in dev
// as well as in production - to look at another channel, run another instance pinned to it at that
// channel's public_url. A dev-only cookie override is deliberately absent: it would do nothing on
// a force-static route (which is every pretty URL), so a preview would look like it worked while
// rendering the pinned channel
export { DEFAULT_CHANNEL_ID } from "@/app/lib/channel";

// From the [locale] route segment, never from a cookie. A cookie can't be read on the routes that
// serve most of the site (they render without a request), so a cookie-driven language silently
// served everyone the default one - the segment is part of the cache key instead, so each language
// gets its own cached page
export async function getCurrentLocale(): Promise<string> {
  return getLocale();
}

// Not async-cookie-dependent on purpose - the channel is a constant for the process, which is what
// lets force-static routes render correctly. Anything request-scoped here is silently wrong there
export async function getCurrentChannelId(): Promise<string> {
  return requireChannelId();
}
