// Server-side search helpers


import { perRequest, rethrowControlFlow } from "../request";
import { gqlFetch } from "../graphql";
import { SearchBlogPostsDocument, SearchOverviewDocument, SearchProductsDocument } from "./operations.gql";
import type { SearchOverview } from "./overview";

export type { OverviewProduct, SearchOverview } from "./overview";

const OVERVIEW_TIMEOUT_MS = 6000;

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

export const searchProducts = perRequest(
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

export const searchBlogPosts = perRequest(
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

// Never cached here: the backend keeps the overview and paces each shopper, and a slow model just means no overview
export const searchOverview = perRequest(async (query: string): Promise<SearchOverview | null> => {
  const q = query.trim();
  if (!q) return null;
  try {
    const res = await gqlFetch(SearchOverviewDocument, { query: q }, { signal: AbortSignal.timeout(OVERVIEW_TIMEOUT_MS) });
    const o = res.data.searchOverview;
    if (!o?.text.trim()) return null;
    return { text: o.text, recap: o.recap, products: o.products.map(({ id, name, url }) => ({ id, name, url })) };
  } catch (err) {
    rethrowControlFlow(err);
    return null;
  }
});
