
import { notFound, perRequest } from "../request";
import { getChannel } from "./read";
import { getCurrentChannelId } from "../locale/read";
import { SITE_URL, trimOrigin } from "../site-url";

export const getSiteUrl = perRequest(async (): Promise<string> => {
  const url = (await getChannel())?.publicUrl?.trim();
  return url && /^https?:\/\//.test(url) ? trimOrigin(url) : SITE_URL;
});

// The root routes outside [channel] (sitemaps, robots.txt) answer a host no channel claims with a 404
export async function requireRequestChannel(): Promise<void> {
  if (!(await getCurrentChannelId())) notFound();
}
