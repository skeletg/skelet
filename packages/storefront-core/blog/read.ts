// Server-side blog queries


import { perRequest } from "../request";
import {
  BlogAuthorByIdDocument,
  BlogAuthorsIndexDocument,
  BlogAuthorBySlugDocument,
  BlogCategoryBySlugDocument,
  BlogPostByIdDocument,
  BlogPostDocument,
  BlogPostsDocument,
  BlogPostsFilteredDocument,
  BlogPostsForSitemapDocument,
  BlogTagBySlugDocument,
  BlogTaxonomyIndexDocument,
  type BlogAuthorBySlugQuery,
  type BlogCategoryBySlugQuery,
  type BlogPostsFilteredQueryVariables,
  type BlogTagBySlugQuery,
} from "./operations.gql";
import { gqlFetch } from "../graphql";
import { resolveRoute } from "../url-resolver/read";

export type BlogAuthor = {
  id: string;
  slug: string;
  displayName: string;
};

export type BlogCategory = {
  id: string;
  slug: string;
  urlPath: string;
  content: { name: string } | null;
};

export type BlogTag = {
  id: string;
  slug: string;
  content: { name: string } | null;
};

export type BlogPostContent = {
  locale: string;
  title: string;
  excerpt: unknown;
  body: unknown;
  metaTitle: string | null;
  metaDescription: string | null;
};

export type BlogPostSummary = {
  id: string;
  slug: string;
  publishedAt: string | null;
  readingTimeMinutes: number | null;
  seoNoindex: boolean;
  author: BlogAuthor | null;
  content: BlogPostContent | null;
  categories: BlogCategory[];
  tags: BlogTag[];
};

export type BlogPostDetail = BlogPostSummary;

// React.cache-wrapped so the same listing/slug fetched from multiple server components
// in one render shares a roundtrip
export const listBlogPosts = perRequest(async (opts: {
  limit?: number;
  offset?: number;
} = {}): Promise<{
  posts: BlogPostSummary[];
  totalCount: number | null;
}> => {
  const res = await gqlFetch(BlogPostsDocument, {
    limit: opts.limit ?? 20,
    offset: opts.offset ?? 0,
  });
  return {
    posts: res.data.blogPosts.items,
    totalCount: res.data.blogPosts.totalCount,
  };
});

export type BlogSitemapEntry = { slug: string; publishedAt: string | null };

export const listBlogPostsForSitemap = perRequest(async (): Promise<BlogSitemapEntry[]> => {
  try {
    const res = await gqlFetch(BlogPostsForSitemapDocument, { limit: 5000, offset: 0 });
    return res.data.blogPosts.items
      .filter((p) => !p.seoNoindex)
      .map((p) => ({ slug: p.slug, publishedAt: p.publishedAt }));
  } catch {
    return [];
  }
});

export const getBlogPost = perRequest(async (slug: string): Promise<{
  post: BlogPostDetail | null;
}> => {
  try {
    const res = await gqlFetch(BlogPostDocument, { slug });
    return { post: res.data.blogPost };
  } catch {
    // The front API wraps "post not found" as the generic INTERNAL error code via its
    // error presenter; there's no SKU-style sentinel we can match on
    return { post: null };
  }
});

export const getBlogPostById = perRequest(async (id: string): Promise<BlogPostDetail | null> => {
  try {
    const res = await gqlFetch(BlogPostByIdDocument, { id });
    return res.data.blogPost;
  } catch {
    return null;
  }
});

// ===== Taxonomy archives ===== Canonical taxonomy URLs

export function blogAuthorHref(a: { slug: string }): string {
  return "/blog/author/" + a.slug;
}

export function blogCategoryHref(c: { urlPath: string }): string {
  return "/" + c.urlPath;
}

export function blogTagHref(t: { slug: string }): string {
  return "/blog/tag/" + t.slug;
}

export type BlogAuthorDetail = NonNullable<BlogAuthorBySlugQuery["blogAuthor"]>;
export type BlogCategoryDetail = NonNullable<BlogCategoryBySlugQuery["blogCategory"]>;
export type BlogTagDetail = NonNullable<BlogTagBySlugQuery["blogTag"]>;

// Same not-found semantics as getBlogPost - the front API reports a missing taxonomy as
// a generic INTERNAL error, so any failure here means "render 404"
export const getBlogAuthor = perRequest(async (slug: string): Promise<BlogAuthorDetail | null> => {
  try {
    const res = await gqlFetch(BlogAuthorBySlugDocument, { slug });
    return res.data.blogAuthor;
  } catch {
    return null;
  }
});

export const getBlogCategory = perRequest(async (slug: string): Promise<BlogCategoryDetail | null> => {
  try {
    const res = await gqlFetch(BlogCategoryBySlugDocument, { slug });
    return res.data.blogCategory;
  } catch {
    return null;
  }
});

export const getBlogTag = perRequest(async (slug: string): Promise<BlogTagDetail | null> => {
  try {
    const res = await gqlFetch(BlogTagBySlugDocument, { slug });
    return res.data.blogTag;
  } catch {
    return null;
  }
});

export const BLOG_ARCHIVE_PAGE_SIZE = 20;

export function parseBlogPage(
  sp: Record<string, string | string[] | undefined>,
): number {
  const raw = sp.page;
  const str = Array.isArray(raw) ? raw[0] : raw;
  if (!str) return 1;
  const n = Number.parseInt(str, 10);
  return Number.isFinite(n) && n >= 1 ? n : 1;
}

export const listBlogPostsBy = perRequest(async (
  filter: BlogPostsFilteredQueryVariables["filter"],
  opts: { limit?: number; offset?: number } = {},
): Promise<{
  posts: BlogPostSummary[];
  hasMore: boolean;
}> => {
  const limit = opts.limit ?? 20;
  const res = await gqlFetch(BlogPostsFilteredDocument, {
    filter,
    limit: limit + 1,
    offset: opts.offset ?? 0,
  });
  const items = res.data.blogPosts.items;
  return {
    posts: items.slice(0, limit),
    hasMore: items.length > limit,
  };
});

export type BlogCategoryLink = {
  id: string;
  slug: string;
  urlPath: string;
  content: { name: string } | null;
  children?: BlogCategoryLink[];
};

export type BlogTagLink = {
  id: string;
  slug: string;
  content: { name: string } | null;
};

export type BlogTaxonomy = {
  categories: BlogCategoryLink[];
  tags: BlogTagLink[];
  categoryNames: Map<string, string>;
  tagNames: Map<string, string>;
};

function flattenCategories(nodes: BlogCategoryLink[]): BlogCategoryLink[] {
  return nodes.flatMap((n) => [n, ...flattenCategories(n.children ?? [])]);
}

export const getBlogTaxonomy = perRequest(async (): Promise<BlogTaxonomy> => {
  const empty: BlogTaxonomy = {
    categories: [],
    tags: [],
    categoryNames: new Map(),
    tagNames: new Map(),
  };
  try {
    const res = await gqlFetch(BlogTaxonomyIndexDocument, {});
    const categories = flattenCategories(res.data.blogCategories);
    const tags = res.data.blogTags;
    return {
      categories,
      tags,
      categoryNames: new Map(
        categories.flatMap((c) => (c.content ? [[c.slug, c.content.name] as const] : [])),
      ),
      tagNames: new Map(
        tags.flatMap((t) => (t.content ? [[t.slug, t.content.name] as const] : [])),
      ),
    };
  } catch {
    return empty;
  }
});

export async function blogTaxonomyRedirect(path: string): Promise<string | null> {
  const r = await resolveRoute(path);
  if (!r?.redirectTo) return null;
  return r.redirectTo.startsWith("/") ? r.redirectTo : "/" + r.redirectTo;
}

// Canonical archive path for a taxonomy entity id
export async function blogTaxonomyHrefById(
  kind: "BLOG_CATEGORY" | "BLOG_TAG" | "BLOG_AUTHOR",
  entityId: string,
): Promise<string | null> {
  try {
    if (kind === "BLOG_AUTHOR") {
      const res = await gqlFetch(BlogAuthorByIdDocument, { id: entityId });
      const a = res.data.blogAuthor;
      return a ? blogAuthorHref(a) : null;
    }
    const tax = await getBlogTaxonomy();
    if (kind === "BLOG_CATEGORY") {
      const c = tax.categories.find((x) => x.id === entityId);
      return c ? blogCategoryHref(c) : null;
    }
    const t = tax.tags.find((x) => x.id === entityId);
    return t ? blogTagHref(t) : null;
  } catch {
    return null;
  }
}

export const listBlogAuthorsForSitemap = perRequest(
  async (): Promise<{ slug: string }[]> => {
    try {
      const res = await gqlFetch(BlogAuthorsIndexDocument, {});
      return res.data.blogAuthors;
    } catch {
      return [];
    }
  },
);
