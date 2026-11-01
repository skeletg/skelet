import type { APIRoute } from "astro";

import { CATALOG_SURFACE } from "@skelet/storefront-core/catalog/read";
import { cacheCatalog } from "../lib/cache";
import { getSite } from "../lib/channel";
import { gql, PUBLIC } from "../lib/gql";
import { HomeProductsDocument } from "../graphql/product-list.gql";
import { absolute, sitemapXml } from "../lib/seo";

// Every product the home page lists, so it shares the home page's purge tag
export const GET: APIRoute = async ({ url, cache }) => {
  await cacheCatalog(cache, CATALOG_SURFACE, ["catalog:browse"]);
  const [{ browseProducts }, { origin }] = await Promise.all([gql(HomeProductsDocument, {}, PUBLIC), getSite(url.origin)]);
  const urls = [absolute(origin, "/"), ...browseProducts.items.flatMap((p) => (p.url ? [absolute(origin, p.url)] : []))];
  return new Response(sitemapXml(urls), { headers: { "Content-Type": "application/xml; charset=utf-8" } });
};
