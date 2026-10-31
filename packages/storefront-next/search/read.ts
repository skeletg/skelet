// Server-side search helpers

import { cache } from "react";

import { gqlFetch } from "../graphql";
import { SearchBlogPostsDocument, SearchProductsDocument } from "./operations.gql";

export type ProductSearchHit = {
  id: string;
  url: string;
  sku: string;
  name: string | null;
};

export type BlogPostSearchHit = {
  id: string;
  slug: string;
  title: string | null;
  excerpt: string | null;
  publishedAt: string | null;
  authorName: string | null;
};

export const searchProducts = cache(
  async (query: string, opts: { limit?: number; offset?: number } = {}): Promise<{
    items: ProductSearchHit[];
    totalCount: number;
    redirectUrl?: string | null;
  }> => {
    if (!query.trim()) return { items: [], totalCount: 0 };
    try {
      const res = await gqlFetch(SearchProductsDocument, {
        query,
        limit: opts.limit ?? 20,
        offset: opts.offset ?? 0,
      });
      return {
        items: res.data.searchProducts.items,
        totalCount: res.data.searchProducts.totalCount,
        redirectUrl: res.data.searchProducts.redirectUrl,
      };
    } catch {
      // OpenSearch is a soft dependency on the backend too - keep the UI rendering even
      // if the cluster is briefly down
      return { items: [], totalCount: 0 };
    }
  },
);

export const searchBlogPosts = cache(
  async (query: string, opts: { limit?: number; offset?: number } = {}): Promise<{
    items: BlogPostSearchHit[];
    totalCount: number;
  }> => {
    if (!query.trim()) return { items: [], totalCount: 0 };
    try {
      const res = await gqlFetch(SearchBlogPostsDocument, {
        query,
        limit: opts.limit ?? 20,
        offset: opts.offset ?? 0,
      });
      return {
        items: res.data.searchBlogPosts.items,
        totalCount: res.data.searchBlogPosts.totalCount,
      };
    } catch {
      return { items: [], totalCount: 0 };
    }
  },
);
