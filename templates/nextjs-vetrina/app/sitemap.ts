import type { MetadataRoute } from "next";

import { requireRequestChannel } from "@skelet/storefront-next/channel/site";
import { getSeoConfig } from "@skelet/storefront-next/config/read";
import { absUrl, applySitemapExclusions, getSiteUrl } from "@skelet/storefront-next/sitemap/read";
import { getHomePage, listCMSPagesForSitemap } from "@skelet/storefront-next/cms/read";
import {
  blogAuthorHref,
  blogCategoryHref,
  blogTagHref,
  getBlogTaxonomy,
  listBlogAuthorsForSitemap,
  listBlogPostsForSitemap,
} from "@skelet/storefront-next/blog/read";

// Per request: the host decides the channel, and so the origin and the entries
export const dynamic = "force-dynamic";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  await requireRequestChannel();
  const [seo, base] = await Promise.all([getSeoConfig(), getSiteUrl()]);
  const abs = (path: string) => absUrl(path, base);
  // Skip the fetches for whole feeds the operator turned off, so a disabled section
  // costs nothing rather than being built and then filtered out
  const [home, cmsPages, blogPosts, taxonomy, authors] = await Promise.all([
    getHomePage(),
    seo.cms ? listCMSPagesForSitemap() : Promise.resolve([]),
    seo.blog ? listBlogPostsForSitemap() : Promise.resolve([]),
    seo.blog ? getBlogTaxonomy() : Promise.resolve({ categories: [], tags: [] }),
    seo.blog ? listBlogAuthorsForSitemap() : Promise.resolve([]),
  ]);

  const entries: MetadataRoute.Sitemap = [
    { url: abs("/"), changeFrequency: "daily", priority: 1 },
  ];

  for (const p of cmsPages) {
    // The channel homepage is served at "/" (the catch-all 301s its own path there) -
    // don't also advertise it at its url_path
    if (home && p.id === home.id) continue;
    entries.push({
      url: abs("/" + p.urlPath),
      lastModified: p.publishedAt ? new Date(p.publishedAt) : undefined,
      changeFrequency: "weekly",
    });
  }

  for (const post of blogPosts) {
    entries.push({
      url: abs("/blog/" + post.slug),
      lastModified: post.publishedAt ? new Date(post.publishedAt) : undefined,
      changeFrequency: "monthly",
    });
  }

  for (const c of taxonomy.categories) {
    entries.push({ url: abs(blogCategoryHref(c)), changeFrequency: "weekly" });
  }
  for (const t of taxonomy.tags) {
    entries.push({ url: abs(blogTagHref(t)), changeFrequency: "weekly" });
  }
  for (const a of authors) {
    entries.push({ url: abs(blogAuthorHref(a)), changeFrequency: "weekly" });
  }

  // Operator extras get their own entries, appended verbatim (relative paths absolutised)
  for (const u of seo.extraUrls) {
    entries.push({ url: abs(u), changeFrequency: "weekly" });
  }

  return applySitemapExclusions(entries, base);
}
