import { notFound } from "next/navigation";
import { cache } from "react";

import { getChannel } from "@/app/lib/channel/read";
import { getCurrentChannelId } from "@/app/lib/locale/read";
import { SITE_URL, trimOrigin } from "@/app/lib/site-url";

export const getSiteUrl = cache(async (): Promise<string> => {
  const url = (await getChannel())?.publicUrl?.trim();
  return url && /^https?:\/\//.test(url) ? trimOrigin(url) : SITE_URL;
});

// The root routes outside [channel] (sitemaps, robots.txt) answer a host no channel claims with a 404
export async function requireRequestChannel(): Promise<void> {
  if (!(await getCurrentChannelId())) notFound();
}
