import type { MetadataRoute } from "next";

import { getSeoConfig } from "@/app/lib/config/read";
import {
  absUrl,
  applySitemapExclusions,
  chunkCount,
  getSitemapCounts,
  listSitemapEntries,
} from "@/app/lib/sitemap/read";

export const revalidate = 3600;

export async function generateSitemaps() {
  const [counts, seo] = await Promise.all([getSitemapCounts(), getSeoConfig()]);
  return Array.from({ length: chunkCount(counts.collections, seo.maxUrlsPerFile) }, (_, i) => ({ id: i }));
}

export default async function sitemap(props: { id: Promise<string> }): Promise<MetadataRoute.Sitemap> {
  // Guard the direct hit too - the index already drops this feed when off
  const seo = await getSeoConfig();
  if (!seo.collections) return [];
  const id = Number(await props.id);
  const entries = await listSitemapEntries("COLLECTION", Number.isFinite(id) ? id : 0, seo.maxUrlsPerFile);
  return applySitemapExclusions(
    entries.map((e) => ({
      url: absUrl("/" + e.path),
      lastModified: new Date(e.updatedAt),
      changeFrequency: "weekly" as const,
    })),
  );
}
