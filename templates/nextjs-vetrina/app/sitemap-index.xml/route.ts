import { getSeoConfig } from "@skelet/storefront-next/config/read";
import { requireRequestChannel } from "@skelet/storefront-next/channel/site";
import { absUrl, chunkCount, getSiteUrl, getSitemapCounts } from "@skelet/storefront-next/sitemap/read";

export const dynamic = "force-dynamic";

export async function GET() {
  await requireRequestChannel();
  const [counts, seo, base] = await Promise.all([getSitemapCounts(), getSeoConfig(), getSiteUrl()]);
  const abs = (path: string) => absUrl(path, base);
  // The root sitemap (home + CMS + blog) is always listed; each catalog feed appears
  // only when its toggle is on, so a disabled section drops out of the index entirely
  const urls: string[] = [abs("/sitemap.xml")];
  const size = seo.maxUrlsPerFile;
  if (seo.products) for (let i = 0; i < chunkCount(counts.products, size); i++) urls.push(abs(`/products/sitemap/${i}.xml`));
  if (seo.categories) for (let i = 0; i < chunkCount(counts.categories, size); i++) urls.push(abs(`/categories/sitemap/${i}.xml`));
  if (seo.collections) for (let i = 0; i < chunkCount(counts.collections, size); i++) urls.push(abs(`/collections/sitemap/${i}.xml`));

  const xml =
    `<?xml version="1.0" encoding="UTF-8"?>\n` +
    `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n` +
    urls.map((u) => `  <sitemap><loc>${u}</loc></sitemap>`).join("\n") +
    `\n</sitemapindex>\n`;

  return new Response(xml, {
    headers: { "content-type": "application/xml; charset=utf-8" },
  });
}
