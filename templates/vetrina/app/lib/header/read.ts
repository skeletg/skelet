import { cache } from "react";

import { gqlFetch } from "@/app/lib/graphql";
import { surfaceCacheOpts, NAV_SURFACE, NAV_TAG } from "@/app/lib/config/read";
import { HeaderDocument } from "@/app/lib/header/operations.gql";

export type HeaderCategory = {
  id: string;
  slug: string;
  urlPath: string;
  name: string | null;
  children: HeaderCategory[];
};

export type HeaderCollection = {
  id: string;
  slug: string;
  name: string | null;
};

export type HeaderLocale = {
  code: string;
  name: string;
  nativeName: string;
  flagEmoji: string;
  // The tag to render in `lang`/hreflang - codes are stored lowercase, so this is where an
  // operator supplies the conventional form ("de-DE", "zh-Hans") - "" falls back to the code
  hreflangValue: string;
};

export const loadHeader = cache(async (): Promise<{
  locales: HeaderLocale[];
  categories: HeaderCategory[];
  collections: HeaderCollection[];
}> => {
  const cacheOpts = await surfaceCacheOpts(NAV_SURFACE, { tags: [NAV_TAG] });
  const { data } = await gqlFetch(HeaderDocument, {}, cacheOpts);
// Flattens every root's direct children in root order - most stores have one root
// hydrateCategory normalises nullable children so the menu can recurse without guards
  const categories = data.rootCategories.flatMap((r) =>
    (r.children ?? []).map(hydrateCategory),
  );
  return {
    locales: data.currentChannel?.locales ?? [],
    categories,
    collections: data.collections,
  };
});

// hydrateCategory ensures every level of the tree carries a well-typed `children` array
function hydrateCategory(c: {
  id: string;
  slug: string;
  urlPath: string;
  name: string | null;
  children?: Array<{
    id: string;
    slug: string;
    urlPath: string;
    name: string | null;
    children?: Array<{
      id: string;
      slug: string;
      urlPath: string;
      name: string | null;
    }>;
  }>;
}): HeaderCategory {
  const kids = c.children ?? [];
  return {
    id: c.id,
    slug: c.slug,
    urlPath: c.urlPath,
    name: c.name,
    children: kids.map((k) => ({
      id: k.id,
      slug: k.slug,
      urlPath: k.urlPath,
      name: k.name,
      children: (k.children ?? []).map((kk) => ({
        id: kk.id,
        slug: kk.slug,
        urlPath: kk.urlPath,
        name: kk.name,
        children: [],
      })),
    })),
  };
}
