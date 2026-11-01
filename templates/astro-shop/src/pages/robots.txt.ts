import type { APIRoute } from "astro";

import { CONFIG_TAG, NAV_TAG } from "@skelet/storefront-core/config/read";

import { getSite } from "../lib/channel";
import { absolute } from "../lib/seo";

export const GET: APIRoute = async ({ url, cache }) => {
  cache.set({ maxAge: 86400, tags: [NAV_TAG, CONFIG_TAG] });
  const { origin } = await getSite(url.origin);
  const body = ["User-agent: *", "Allow: /", "Disallow: /cart", "Disallow: /checkout", "Disallow: /order-placed", "Disallow: /api/", "", `Sitemap: ${absolute(origin, "/sitemap.xml")}`, ""].join("\n");
  return new Response(body, { headers: { "Content-Type": "text/plain; charset=utf-8" } });
};
