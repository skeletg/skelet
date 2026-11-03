// Builds Next Metadata (title, description, canonical, Open Graph, Twitter Card,
// robots) for a CMS page
import type { Metadata } from "next";

import { SITE_URL, absUrl } from "@skelet/storefront-core/site-url";

// Structural on purpose - both CMSPage and the leaner HomePage shape satisfy it
type CMSPageForMetadata = {
  slug: string;
  seoNoindex: boolean;
  seoNofollow: boolean;
  canonicalUrl: string | null;
  ogImage: string | null;
  content: {
    title: string;
    metaTitle: string | null;
    metaDescription: string | null;
  } | null;
};

export function cmsPageMetadata(page: CMSPageForMetadata, href: string, base: string = SITE_URL): Metadata {
  const abs = (path: string) => absUrl(path, base);
  const c = page.content;
  const title = c?.metaTitle?.trim() || c?.title?.trim() || page.slug;
  const description = c?.metaDescription?.trim() || undefined;
  const ogImage = page.ogImage ? abs(page.ogImage) : undefined;
  // Operator override wins, else the page self-canonicalises to its own URL
  const canonical = page.canonicalUrl?.trim() || href;

  const meta: Metadata = {
    title,
    description,
    alternates: { canonical },
    openGraph: {
      type: "website",
      title,
      description,
      url: abs(canonical),
      images: ogImage ? [{ url: ogImage }] : undefined,
    },
    twitter: {
      card: ogImage ? "summary_large_image" : "summary",
      title,
      description,
      images: ogImage ? [ogImage] : undefined,
    },
  };
  // index and follow are independent: noindex does not force nofollow, so a
  // hidden page can still pass link equity unless nofollow is also set
  const index = !page.seoNoindex;
  const follow = !page.seoNofollow;
  if (!index || !follow) meta.robots = { index, follow };
  return meta;
}
