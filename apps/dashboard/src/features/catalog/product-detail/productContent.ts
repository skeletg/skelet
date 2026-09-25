import { useApolloClient, useMutation } from '@apollo/client/react'
import type { UpsertProductContentInput } from '@/graphql/schema.types'
import { ProductDocument, UpsertProductContentDocument } from '../Catalog.gql'

// upsertProductContent replaces the locale's whole row, while the General, Content and SEO sections
// each own a slice of it. So every write sends the full row: the row as it is when the write runs,
// read from the cache (where a section that saved a moment earlier in the same Save already put its
// part), with this section's fields on top
export function useProductContentWrite(productId: string, locale: string) {
  const client = useApolloClient()
  const variables = { id: productId }
  const [upsert] = useMutation(UpsertProductContentDocument, {
    refetchQueries: ['ProductsPaged', { query: ProductDocument, variables }],
  })
  return async (patch: Partial<UpsertProductContentInput>) => {
    // Partial is enough - only the name and the content rows are needed
    const product = client.cache.readQuery({ query: ProductDocument, variables, returnPartialData: true })?.product
    const row = product?.content?.find((c) => c.locale === locale)
    const input: UpsertProductContentInput = {
      name: row?.name ?? product?.name ?? product?.content?.[0]?.name ?? 'Untitled product',
      shortDescription: row?.shortDescription ?? undefined,
      descriptionRichText: row?.descriptionRichText ?? undefined,
      metaTitle: row?.metaTitle ?? undefined,
      metaDescription: row?.metaDescription ?? undefined,
      metaKeywords: row?.metaKeywords ?? undefined,
      ...patch,
    }
    await upsert({ variables: { productId, locale, input } })
  }
}
