import type { RecommendationProductsQuery } from '../Catalog.gql'

export type RecommendationProduct = RecommendationProductsQuery['productsByIds'][number]
export const recommendationName = (p?: RecommendationProduct) => p?.name || p?.sku || 'Unavailable product'
export const recommendationSummary = (p?: RecommendationProduct) => p?.name && p.sku ? `${p.name} (${p.sku})` : recommendationName(p)

