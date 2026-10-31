// Server-side CMS page reader

import { cache } from "react";

import {
  CmsPageByIdDocument,
  CmsPagesForSitemapDocument,
  HomePageDocument,
} from "@/app/lib/cms/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";
import { getAccessToken } from "@/app/lib/auth/session";
import { surfaceCacheOpts } from "@/app/lib/config/read";
import { getCurrentChannelId } from "@/app/lib/locale/read";

// The "cms" cache surface key - matches the backend revalidate.Surface and the admin
// Cache page
const CMS_SURFACE = "cms";

export type CMSPageContent = {
  locale: string;
  title: string;
  body: unknown;
  metaTitle: string | null;
  metaDescription: string | null;
};

export type CMSPageVariant = { variantKey: string; body: unknown };

export type CMSPage = {
  id: string;
  slug: string;
  urlPath: string;
  isPublished: boolean;
  seoNoindex: boolean;
  // robots nofollow directive (seo.nofollow metadata) - pairs with seoNoindex
  seoNofollow: boolean;
  // operator rel=canonical override (seo.canonical metadata) - null → self-canonical
  canonicalUrl: string | null;
  // raw operator-authored JSON-LD (seo.jsonld metadata) - null → none, validated before injection
  structuredData: string | null;
  // builder spec (seo.schema metadata) the storefront turns into fresh JSON-LD - raw structuredData wins over it
  structuredDataSpec: string | null;
  publishedAt: string | null;
  // Social-share image (operator-set seo.ogImage metadata, else the first body
  // image), resolved server-side. Null = none → og:image is omitted
  ogImage: string | null;
  experimentKey: string;
  variants: CMSPageVariant[];
  content: CMSPageContent | null;
};

export const getCMSPage = cache(async (id: string): Promise<CMSPage | null> => {
  try {
    // Cache only when the CMS surface is admin-enabled AND the visitor is anonymous (logged-in visitors may get
    // segment-personalized blocks filtered server-side, so their response must not be shared)
    const cacheOpts = await surfaceCacheOpts(CMS_SURFACE, {
      tags: [`cms:page:${id}`, "cms:all"],
      token: await getAccessToken(),
    });
    const res = await gqlFetch(CmsPageByIdDocument, { id }, cacheOpts);
    return res.data.cmsPage;
  } catch {
    return null;
  }
});

export type HomePage = {
  id: string;
  slug: string;
  seoNoindex: boolean;
  seoNofollow: boolean;
  canonicalUrl: string | null;
  structuredData: string | null;
  // builder spec (seo.schema metadata) - raw structuredData wins over it
  structuredDataSpec: string | null;
  publishedAt: string | null;
  ogImage: string | null;
  content: {
    title: string;
    body: unknown;
    metaTitle: string | null;
    metaDescription: string | null;
  } | null;
};

export const getHomePage = cache(async (): Promise<HomePage | null> => {
  try {
    const [channelId, token] = await Promise.all([
      getCurrentChannelId(),
      getAccessToken(),
    ]);
    // The homepage fetch isn't keyed by page id; bust via cms:home:<channel> (or the
    // coarse cms:all / revalidatePath("/")) on publish
    const cacheOpts = await surfaceCacheOpts(CMS_SURFACE, {
      tags: [`cms:home:${channelId}`, "cms:all"],
      token,
    });
    const res = await gqlFetch(HomePageDocument, {}, cacheOpts);
    return res.data.homePage;
  } catch {
    return null;
  }
});

export type CMSSitemapEntry = { id: string; urlPath: string; publishedAt: string | null };

export const listCMSPagesForSitemap = cache(async (): Promise<CMSSitemapEntry[]> => {
  try {
    const channelId = await getCurrentChannelId();
    const cacheOpts = await surfaceCacheOpts(CMS_SURFACE, {
      tags: [`cms:sitemap:${channelId}`, "cms:all"],
      keyExtra: [channelId],
    });
    const res = await gqlFetch(CmsPagesForSitemapDocument, { limit: 5000, offset: 0 }, cacheOpts);
    return res.data.cmsPages
      .filter((p) => !p.seoNoindex && !!p.urlPath)
      .map((p) => ({ id: p.id, urlPath: p.urlPath, publishedAt: p.publishedAt }));
  } catch {
    return [];
  }
});
