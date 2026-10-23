import type { RecommendationProductsQuery } from '../Catalog.gql'

export type RecommendationProduct = RecommendationProductsQuery['productsByIds'][number]
export const recommendationName = (p?: RecommendationProduct) => p?.name || p?.sku || 'Unavailable product'
export const recommendationSummary = (p?: RecommendationProduct) => p?.name && p.sku ? `${p.name} (${p.sku})` : recommendationName(p)


// Past this a list scrolls inside its own frame and offers a filter
export const LONG_LIST = 10

export const matchesRecommendation = (p: RecommendationProduct | undefined, query: string) => {
  const q = query.trim().toLowerCase()
  return q === '' || `${recommendationName(p)} ${p?.sku ?? ''}`.toLowerCase().includes(q)
}
