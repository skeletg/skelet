import { revalidatePath } from "next/cache";

import { channelRoutePaths } from "./channel-routing";
import {
  getCurrentChannelId,
  getCurrentChannelLocales,
  getCurrentLocale,
} from "./locale/read";

// revalidatePath matches the route file a page renders from, not the URL the shopper sees - and the proxy rewrites
// every public path to /<channel>/<locale>/..., so a bare revalidatePath("/some-product") matched nothing and the
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
  for (const route of channelRoutePaths(path, channelId, locales)) revalidatePath(route);
}

// Everything under the account layout - the overview, the nav and the page that changed all read the same customer
export function revalidateAccount(): void {
  revalidatePath("/[channel]/[locale]/account", "layout");
}
