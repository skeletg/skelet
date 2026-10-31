import type { MetadataRoute } from "next";

import { requireRequestChannel } from "@/app/lib/channel/site";
import { getSeoConfig } from "@/app/lib/config/read";
import {
  absUrl,
  applySitemapExclusions,
  chunkCount,
  getSitemapCounts,
  getSiteUrl,
  listSitemapEntries,
} from "@/app/lib/sitemap/read";

export const dynamic = "force-dynamic";

export async function generateSitemaps() {
  const [counts, seo] = await Promise.all([getSitemapCounts(), getSeoConfig()]);
  return Array.from({ length: chunkCount(counts.categories, seo.maxUrlsPerFile) }, (_, i) => ({ id: i }));
}

export default async function sitemap(props: { id: Promise<string> }): Promise<MetadataRoute.Sitemap> {
  await requireRequestChannel();
  // Guard the direct hit too - the index already drops this feed when off
  const [seo, base] = await Promise.all([getSeoConfig(), getSiteUrl()]);
  if (!seo.categories) return [];
  const id = Number(await props.id);
  const entries = await listSitemapEntries("CATEGORY", Number.isFinite(id) ? id : 0, seo.maxUrlsPerFile);
  return applySitemapExclusions(
    entries.map((e) => ({
      url: absUrl("/" + e.path, base),
      lastModified: new Date(e.updatedAt),
      changeFrequency: "weekly" as const,
    })),
    base,
  );
}
