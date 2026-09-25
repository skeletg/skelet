import { useApolloClient, useMutation } from '@apollo/client/react'
import type { SetProductListingInput } from '@/graphql/schema.types'
import { ProductDocument, SetProductListingDocument } from '../Catalog.gql'

type ListingPatch = Partial<Pick<SetProductListingInput, 'isPublished' | 'price' | 'url'>>

// setProductListing replaces a store's whole listing, while the per-store pricing & publishing
// section and the per-store URL section each own part of it. So every write sends the full listing:
// the listing as it is when the write runs, read from the cache (where a section that saved a moment
// earlier in the same Save already put its part), with this section's fields on top
export function useProductListingWrite(productId: string) {
  const client = useApolloClient()
  const variables = { id: productId }
  const [setListing] = useMutation(SetProductListingDocument, {
    refetchQueries: [{ query: ProductDocument, variables }],
  })
  return async (channelId: string, patch: ListingPatch) => {
    // Partial is enough - only the listings are needed
    const product = client.cache.readQuery({ query: ProductDocument, variables, returnPartialData: true })?.product
    const current = product?.listings?.find((l) => l.channel?.id === channelId)
    await setListing({
      variables: {
        input: {
          productId,
          channelId,
          isPublished: current?.isPublished ?? false,
          price: current?.price ?? null,
          url: current?.url || null,
          ...patch,
        },
      },
    })
  }
}
