import { getSeoConfig } from "@/app/lib/config/read";
import { absUrl, chunkCount, getSitemapCounts } from "@/app/lib/sitemap/read";

export const revalidate = 3600;

export async function GET() {
  const [counts, seo] = await Promise.all([getSitemapCounts(), getSeoConfig()]);
  // The root sitemap (home + CMS + blog) is always listed; each catalog feed appears
  // only when its toggle is on, so a disabled section drops out of the index entirely
  const urls: string[] = [absUrl("/sitemap.xml")];
  const size = seo.maxUrlsPerFile;
  if (seo.products) for (let i = 0; i < chunkCount(counts.products, size); i++) urls.push(absUrl(`/products/sitemap/${i}.xml`));
  if (seo.categories) for (let i = 0; i < chunkCount(counts.categories, size); i++) urls.push(absUrl(`/categories/sitemap/${i}.xml`));
  if (seo.collections) for (let i = 0; i < chunkCount(counts.collections, size); i++) urls.push(absUrl(`/collections/sitemap/${i}.xml`));

  const xml =
    `<?xml version="1.0" encoding="UTF-8"?>\n` +
    `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n` +
    urls.map((u) => `  <sitemap><loc>${u}</loc></sitemap>`).join("\n") +
    `\n</sitemapindex>\n`;

  return new Response(xml, {
    headers: { "content-type": "application/xml; charset=utf-8" },
  });
}
