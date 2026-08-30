// SYNC: mirrors frontCacheableOps in apps/backend/internal/wiring/response_cache.go - the backend serves exactly these from Valkey for anonymous callers, so a name here that isn't there means one side caches what the other won't
// Sending a token on these bypasses that cache AND forces the route dynamic, which is what blocks full-page caching
// Opt-OUT on purpose - a query missing from this list keeps its token, so forgetting one costs performance, never correctness
export const PUBLIC_OPS: ReadonlySet<string> = new Set([
  "Product",
  "ProductWithReviews",
  "ProductsByIds",
  "BrowseProducts",
  "SearchProducts",
  "ProductRecommendations",
  "ProductQuestions",
  "ProductSortOptions",
  "Category",
  "Collection",
  "Collections",
  "Menus",
  "Header",
  "HomePage",
  "Route",
  "CurrentChannel",
  "PublicConfig",
  "StoreFeatures",
  "ConsentDefinitions",
  "Countries",
  "CountrySettings",
  "Regions",
  "BlogPost",
  "BlogPostById",
  "BlogPosts",
  "SearchBlogPosts",
  "CmsPageById",
  "SynapseAttributes",
  "SitemapCounts",
  "SitemapEntries",
  "PresentmentCurrencies",
  "BlogPostsForSitemap",
  "CmsPagesForSitemap",
  "PaymentMethods",
])

export function isPublicOp(opName: string): boolean {
  return opName !== "" && PUBLIC_OPS.has(opName)
}
