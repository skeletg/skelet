import { useApolloClient, useMutation } from '@apollo/client/react'
import type { ProductKind, SetProductListingInput } from '@/graphql/schema.types'
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

export type WrapperContents = {
  kind?: ProductKind
  children?: readonly unknown[]
  bundleOptions?: readonly { selections: readonly unknown[] }[]
  groupedMembers?: readonly unknown[]
  kitComponents?: readonly unknown[]
}

// A wrapper sells what sits behind it, so one with nothing there can still be published but the storefront shows it
// out of stock - the listings editor warns instead of blocking. Reads the saved product, so items staged in the same
// Save don't count until they land
export function emptyWrapperWarning(p: WrapperContents): string | null {
  switch (p.kind) {
    case 'CONFIGURABLE':
      return p.children?.length ? null : 'This configurable product has no variants, so it will show as out of stock on the storefront.'
    case 'BUNDLE':
      return p.bundleOptions?.some((o) => o.selections.length > 0)
        ? null
        : 'This bundle has no selections, so it will show as out of stock on the storefront.'
    case 'GROUPED':
      return p.groupedMembers?.length ? null : 'This grouped product has no members, so it will show as out of stock on the storefront.'
    case 'KIT':
      return p.kitComponents?.length ? null : 'This kit has no components, so it will show as out of stock on the storefront.'
    default:
      return null
  }
}
