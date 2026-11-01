
import { request } from "./request";
import { channelRoutePaths } from "./channel-routing";
import {
  getCurrentChannelId,
  getCurrentChannelLocales,
  getCurrentLocale,
} from "./locale/read";

// Next's revalidation matches the route file a page renders from, not the URL the shopper sees - and the proxy rewrites
// every public path to /<channel>/<locale>/..., so a bare request().revalidate("/some-product") matched nothing and the
// cached page kept serving until its own revalidate window ran out. These go through the same fan-out the backend's
// revalidate webhook uses (app/api/revalidate)

// One public page, in every locale the current channel serves - a new review shows on the German PDP too
export async function revalidatePublicPath(path: string): Promise<void> {
  const [channelId, { codes }] = await Promise.all([
    getCurrentChannelId(),
    getCurrentChannelLocales(),
  ]);
  // An empty list means the backend didn't answer - still bust the page the shopper is on
  const locales = codes.length > 0 ? codes : [await getCurrentLocale()];
  for (const route of channelRoutePaths(path, channelId, locales)) request().revalidate(route);
}

// Everything under the account layout - the overview, the nav and the page that changed all read the same customer
export function revalidateAccount(): void {
  request().revalidate("/[channel]/[locale]/account", "layout");
}
