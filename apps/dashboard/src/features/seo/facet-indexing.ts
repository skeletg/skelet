import { INPUT_TYPES_WITH_VALUES } from '@/features/catalog/attributeInputTypes'
import type { AttributeInputType } from '@/graphql/schema.types'

// Pure helpers behind the facet-indexing page - split from the component file so fast
// refresh keeps working, same reason attributeInputTypes.ts exists

export type FacetValue = {
  id: string
  name: string
  slug: string
  isIndexable: boolean
}

export type FacetAttr = {
  id: string
  slug: string
  name: string
  inputType: string
  isIndexable: boolean
  h1Position: string
  values: FacetValue[]
}

// Only these input types carry AttributeValue rows - scalars (plain text, numeric,
// boolean, date) and references have no per-value flag at all, their buckets follow the
// master switch, and that flag is baked into the search index rather than read live
export function hasSelectableValues(inputType: string): boolean {
  return INPUT_TYPES_WITH_VALUES.has(inputType as AttributeInputType)
}

// How many single-facet URLs a facet opens up. Null for scalars: their bucket set lives
// in the search index, so the dashboard can't count it
export function indexableValueCount(a: FacetAttr): number | null {
  if (!hasSelectableValues(a.inputType)) return null
  return a.values.filter((v) => v.isIndexable).length
}

// Upper bound on the URLs one combination opens up: the product of its members'
// indexable value counts. Honest ceiling - pairs with no shared products and the
// min-results guard both cut it. Null when a member is scalar (uncountable here) or the
// row isn't a real combination yet
export function combinationEstimate(slugs: string[], attrs: FacetAttr[]): number | null {
  const picked = slugs.filter(Boolean)
  if (picked.length < 2) return null
  let total = 1
  for (const slug of picked) {
    const attr = attrs.find((a) => a.slug === slug)
    const n = attr ? indexableValueCount(attr) : null
    if (n == null) return null
    total *= n
  }
  return total
}
