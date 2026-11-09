import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import { EmailBlockProductsDocument, type EmailBlockProductsQuery } from './Email.gql'

export type BlockProduct = EmailBlockProductsQuery['productsBySkus'][number]

// The products a block names by SKU, keyed by SKU - the panel and the canvas ask with the same SKUs, so it's one fetch
export function useBlockProducts(skus: string[]): { bySku: Map<string, BlockProduct>; loading: boolean } {
  const wanted = skus.map((s) => s.trim()).filter(Boolean)
  const { data, previousData, loading } = useQuery(EmailBlockProductsDocument, {
    variables: { skus: wanted },
    skip: wanted.length === 0,
    fetchPolicy: 'cache-first',
  })
  const rows = (data ?? previousData)?.productsBySkus
  const bySku = useMemo(() => new Map((rows ?? []).map((p) => [p.sku, p])), [rows])
  return { bySku, loading }
}

export function productImage(p: BlockProduct | undefined): string | undefined {
  return p?.thumbnail?.url ?? p?.displayImage?.url ?? undefined
}
