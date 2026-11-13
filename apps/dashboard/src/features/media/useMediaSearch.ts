import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import type { MediaAssetSort, MediaMimeGroup } from '@/graphql/schema.types'
import { SearchMediaAssetsDocument } from './MediaLibrary.gql'

export const MEDIA_PAGE = 48

export type MediaSearch = { query: string; path: string; type: MediaMimeGroup | 'ALL'; sort: MediaAssetSort }

// The library's assets for a search, a page at a time: the picker and the library page both scroll through it.
// The server caps one search at 200, so it pages by offset; a page that brings nothing new (a delete since the
// count) ends it, or the sentinel would ask forever
export function useMediaSearch({ query, path, type, sort }: MediaSearch) {
  const variables = { query: query || null, path: path || null, mimeGroup: type === 'ALL' ? null : type, sort, limit: MEDIA_PAGE, offset: 0 }
  const results = useQuery(SearchMediaAssetsDocument, { variables, fetchPolicy: 'cache-and-network', notifyOnNetworkStatusChange: true })
  const items = useMemo(() => results.data?.searchMediaAssets.items ?? results.previousData?.searchMediaAssets.items ?? [], [results.data, results.previousData])
  const total = results.data?.searchMediaAssets.totalCount ?? items.length

  const pageKey = `${JSON.stringify(variables)}@${items.length}`
  const [stalledAt, setStalledAt] = useState<string | null>(null)
  const hasMore = items.length < total && stalledAt !== pageKey
  const asking = useRef(false)
  const { fetchMore } = results
  const loadMore = useCallback(async () => {
    if (asking.current) return
    asking.current = true
    const seen = new Set(items.map((a) => a.id))
    try {
      const res = await fetchMore({
        variables: { offset: items.length },
        updateQuery: (prev, { fetchMoreResult }) => ({
          searchMediaAssets: {
            ...fetchMoreResult.searchMediaAssets,
            items: [...prev.searchMediaAssets.items, ...fetchMoreResult.searchMediaAssets.items.filter((a) => !seen.has(a.id))],
          },
        }),
      })
      if (!res.data?.searchMediaAssets.items.some((a) => !seen.has(a.id))) setStalledAt(pageKey)
    } catch {
      setStalledAt(pageKey)
    } finally {
      asking.current = false
    }
  }, [fetchMore, items, pageKey])

  const sentinel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = sentinel.current
    if (!el || !hasMore || results.loading) return
    const watch = new IntersectionObserver(([entry]) => entry?.isIntersecting && void loadMore(), { rootMargin: '200px' })
    watch.observe(el)
    return () => watch.disconnect()
  }, [hasMore, results.loading, loadMore])

  return { items, total, loading: results.loading, error: results.error, hasMore, loadMore, sentinel, refetch: results.refetch }
}
