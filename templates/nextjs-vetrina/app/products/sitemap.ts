import type { MetadataRoute } from "next";

import { requireRequestChannel } from "@skelet/storefront-next/channel/site";
import { getSeoConfig } from "@skelet/storefront-next/config/read";
import {
  absUrl,
  applySitemapExclusions,
  chunkCount,
  getSitemapCounts,
  getSiteUrl,
  listSitemapEntries,
} from "@skelet/storefront-next/sitemap/read";

export const dynamic = "force-dynamic";

export async function generateSitemaps() {
  const [counts, seo] = await Promise.all([getSitemapCounts(), getSeoConfig()]);
  return Array.from({ length: chunkCount(counts.products, seo.maxUrlsPerFile) }, (_, i) => ({ id: i }));
}

export default async function sitemap(props: { id: Promise<string> }): Promise<MetadataRoute.Sitemap> {
  await requireRequestChannel();
  // The index already omits this feed when off; guard the direct hit too so a crawler
  // that kept the old URL gets an empty file, not disabled content
  const [seo, base] = await Promise.all([getSeoConfig(), getSiteUrl()]);
  if (!seo.products) return [];
  const id = Number(await props.id);
  const entries = await listSitemapEntries("PRODUCT", Number.isFinite(id) ? id : 0, seo.maxUrlsPerFile);
  return applySitemapExclusions(
    entries.map((e) => ({
      url: absUrl("/" + e.path, base),
      lastModified: new Date(e.updatedAt),
      changeFrequency: "daily" as const,
    })),
    base,
  );
}
