// Entity -> URL. Every Link to a catch-all entity goes through here so hrefs stay canonical with url_rewrites -
// per-entity rules in ARCHITECTURE.md Category/collection helpers take a filter chain to carry /f/../v/.. across links

import { buildFilterPath } from "@/app/lib/filter-path";

type ProductLike = { url: string };
type CategoryLike = { urlPath: string };
type CollectionLike = { slug: string };
type BlogPostLike = { slug: string };
type CMSPageLike = { urlPath: string };
type FilterChain = { slug: string; values: string[] }[];

function withFilterChain(base: string, filters?: FilterChain): string {
  if (!filters || filters.length === 0) return base;
  const chain = buildFilterPath(filters);
  return chain ? `${base}/${chain}` : base;
}

export function productHref(p: ProductLike): string {
  return "/" + p.url;
}

export function categoryHref(c: CategoryLike, filters?: FilterChain): string {
  return withFilterChain("/" + c.urlPath, filters);
}

export function collectionHref(c: CollectionLike, filters?: FilterChain): string {
  return withFilterChain("/" + c.slug, filters);
}

export function blogPostHref(p: BlogPostLike): string {
  return "/blog/" + p.slug;
}

export function cmsPageHref(p: CMSPageLike): string {
  return "/" + p.urlPath;
}
