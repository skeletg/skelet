import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import type { MediaAssetSort, MediaMimeGroup, MediaVisibility } from '@/graphql/schema.types'
import { SearchMediaAssetsDocument } from './MediaLibrary.gql'

export const MEDIA_PAGE = 48

// visibility is left out for the public half, so a search of it is word for word what it always was
export type MediaSearch = { query: string; path: string; type: MediaMimeGroup | 'ALL'; sort: MediaAssetSort; visibility?: MediaVisibility; skip?: boolean }

// The library's assets for a search, a page at a time: the picker and the library page both scroll through it.
// The server caps one search at 200, so it pages by offset; a page that brings nothing new (a delete since the
// count) ends it, or the sentinel would ask forever. A page that fails stops the sentinel too, and waits to be asked again
export function useMediaSearch({ query, path, type, sort, visibility, skip = false }: MediaSearch) {
  const variables = { query: query || null, path: path || null, mimeGroup: type === 'ALL' ? null : type, sort, limit: MEDIA_PAGE, offset: 0, ...(visibility === 'PRIVATE' ? { visibility } : {}) }
  const results = useQuery(SearchMediaAssetsDocument, { variables, skip, fetchPolicy: 'cache-and-network', notifyOnNetworkStatusChange: true })
  // The last answer stands in while the next one comes - but only from the same half, never the other's files under this one's name
  const half = visibility ?? 'PUBLIC'
  const items = useMemo(
    () => results.data?.searchMediaAssets.items ?? results.previousData?.searchMediaAssets.items.filter((a) => a.visibility === half) ?? [],
    [results.data, results.previousData, half],
  )
  const total = results.data?.searchMediaAssets.totalCount ?? items.length

  const pageKey = `${JSON.stringify(variables)}@${items.length}`
  const [stalledAt, setStalledAt] = useState<string | null>(null)
  const [failedAt, setFailedAt] = useState<string | null>(null)
  const hasMore = items.length < total && stalledAt !== pageKey
  const moreFailed = failedAt === pageKey
  const asking = useRef(false)
  const { fetchMore, updateQuery } = results
  const loadMore = useCallback(async () => {
    if (asking.current) return
    asking.current = true
    setFailedAt(null)
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
      setFailedAt(pageKey)
    } finally {
      asking.current = false
    }
  }, [fetchMore, items, pageKey])

  const sentinel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = sentinel.current
    if (!el || !hasMore || moreFailed || results.loading) return
    const watch = new IntersectionObserver(([entry]) => entry?.isIntersecting && void loadMore(), { rootMargin: '200px' })
    watch.observe(el)
    return () => watch.disconnect()
  }, [hasMore, moreFailed, results.loading, loadMore])

  // Takes files off the list where they stand. A refetch would bring back only the first page, and whoever
  // deleted a file far down would be sent back to the top
  const drop = useCallback(
    (ids: string[]) => {
      const gone = new Set(ids)
      updateQuery((_, { complete, previousData }) => {
        if (!complete) return
        const was = previousData.searchMediaAssets
        const kept = was.items.filter((a) => !gone.has(a.id))
        return { searchMediaAssets: { ...was, items: kept, totalCount: Math.max(0, was.totalCount - (was.items.length - kept.length)) } }
      })
    },
    [updateQuery],
  )

  return { items, total, loading: results.loading, error: results.error, hasMore, moreFailed, loadMore, sentinel, drop, refetch: results.refetch }
}
