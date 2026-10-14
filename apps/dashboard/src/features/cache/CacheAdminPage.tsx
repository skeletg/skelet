import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { RotateCcwIcon, TrashIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { LoadingState } from '@/components/ui/loading-state'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import { SetConfigValueDocument } from '@/features/settings/Settings.gql'
import {
  CacheSurfacesDocument,
  CacheRevalidationsDocument,
  PurgeCacheDocument,
  RebuildCacheDocument,
  EnqueueRebuildDocument,
  RebuildQueueDocument,
} from './Cache.gql'
import { formatDateTime } from '@/lib/format'

// CacheAdminPage - the abstract storefront cache console - Lists every registered
// cacheable surface (CMS now; products/categories later) with an enable toggle
// + TTL, manual purge controls (per surface / by tag-path / everything), and a
// log of recent purges - Config edits go through the shared config registry; the
// storefront reads enabled/TTL to decide whether (and how long) to cache
export function CacheAdminPage() {
  const { has } = usePermissions()
  const canManage = has('cache.manage')
  const surfacesQ = useQuery(CacheSurfacesDocument)
  const logQ = useQuery(CacheRevalidationsDocument, { variables: { limit: 50 } })
  const surfaces = surfacesQ.data?.cacheSurfaces ?? []
  const log = logQ.data?.cacheRevalidations ?? []

  const refetch = [
    { query: CacheSurfacesDocument },
    { query: CacheRevalidationsDocument, variables: { limit: 50 } },
  ]
  const queueQ = useQuery(RebuildQueueDocument, { variables: { recentLimit: 10 }, pollInterval: 5000 })
  const queue = queueQ.data?.rebuildQueue
  const [setConfig] = useMutation(SetConfigValueDocument, { refetchQueries: [{ query: CacheSurfacesDocument }] })
  const [purge] = useMutation(PurgeCacheDocument, { refetchQueries: refetch })
  const [rebuild, { loading: rebuilding }] = useMutation(RebuildCacheDocument, { refetchQueries: refetch })
  const [enqueue, { loading: enqueuing }] = useMutation(EnqueueRebuildDocument, {
    refetchQueries: [{ query: RebuildQueueDocument, variables: { recentLimit: 10 } }],
  })

  const [manual, setManual] = useState('')
  const [rebuildPath, setRebuildPath] = useState('')
  const [bulkPaths, setBulkPaths] = useState('')
  const [enqueuedMsg, setEnqueuedMsg] = useState('')

  const setEnabled = (key: string, on: boolean) =>
    setConfig({ variables: { path: `cache/${key}/enabled`, value: on ? 'true' : 'false' } })
  const setTtl = (key: string, ttl: string) =>
    setConfig({ variables: { path: `cache/${key}/ttl_seconds`, value: ttl } })

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/"
        backLabel="Back"
        title="Cache"
        meta={
          <span>
            Storefront cache: per-surface settings, on-demand invalidation, and recent activity.
            Invalidating marks cached pages stale - they rebuild on the next visit, not immediately.
          </span>
        }
      />
      <DetailPageBody>
        <DetailPageSection
          title="Surfaces"
          description="Each cacheable storefront surface. Toggle caching, set its time-to-live, or invalidate it (its pages rebuild on next request)."
        >
          {surfacesQ.loading ? (
            <LoadingState />
          ) : surfaces.length === 0 ? (
            <p className="text-sm text-muted-foreground">No cacheable surfaces registered.</p>
          ) : (
            <ul className="grid gap-2">
              {surfaces.map((s) => (
                <li key={s.key} className="flex flex-wrap items-center gap-3 rounded-md border p-2.5">
                  <div className="min-w-0 flex-1">
                    <span className="block text-sm font-medium">{s.label}</span>
                    <code className="text-xs text-muted-foreground">{s.key}</code>
                  </div>
                  <label className="flex items-center gap-2 text-xs text-muted-foreground">
                    Cached
                    <Switch
                      checked={s.enabled}
                      disabled={!canManage}
                      onCheckedChange={(v) => void setEnabled(s.key, v)}
                    />
                  </label>
                  <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    TTL
                    <Input
                      type="number"
                      min={0}
                      defaultValue={s.ttlSeconds}
                      disabled={!canManage}
                      className="h-8 w-24"
                      onBlur={(e) => {
                        const v = e.target.value.trim()
                        if (v && Number(v) !== s.ttlSeconds) void setTtl(s.key, v)
                      }}
                    />
                    s
                  </label>
                  {canManage &&
                    (s.bulkInvalidatable ? (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => void purge({ variables: { surfaceKey: s.key } })}
                        title="Mark this surface's cached pages stale (they rebuild on next request)"
                      >
                        <RotateCcwIcon className="size-4" /> Invalidate
                      </Button>
                    ) : (
                      <span
                        className="text-xs text-muted-foreground"
                        title="This surface is invalidated per-item only - no catalog-wide bust (avoids overloading the renderer)"
                      >
                        per-item only
                      </span>
                    ))}
                </li>
              ))}
            </ul>
          )}
        </DetailPageSection>

        {canManage && (
          <DetailPageSection
            title="Invalidate cache"
            description="Invalidate a specific cache tag or path, or every registered surface. This marks cached entries stale - affected pages are rebuilt lazily on their next request, not immediately, so there's no mass rebuild."
          >
            <div className="flex flex-wrap items-center gap-2">
              <Input
                value={manual}
                onChange={(e) => setManual(e.target.value)}
                placeholder="cache tag (e.g. cms:all) or path (/about)"
                className="h-9 max-w-sm"
              />
              <Button
                type="button"
                variant="outline"
                disabled={!manual.trim()}
                onClick={() => {
                  const v = manual.trim()
                  const vars = v.startsWith('/') ? { paths: [v] } : { tags: [v] }
                  void purge({ variables: vars })
                  setManual('')
                }}
              >
                Invalidate
              </Button>
              <Button
                type="button"
                variant="destructive"
                onClick={() => void purge({ variables: {} })}
                title="Invalidate all bulk-invalidatable surfaces (per-item-only surfaces like products are excluded). Pages rebuild on next request."
              >
                <TrashIcon className="size-4" /> Invalidate everything
              </Button>
            </div>
          </DetailPageSection>
        )}

        {canManage && (
          <DetailPageSection
            title="Rebuild a path on demand"
            description="Invalidate a storefront path and immediately re-render it so it's warm now - no waiting for the next visitor. (To rebuild a CMS page, the page editor's 'Rebuild now' is more precise.)"
          >
            <div className="flex flex-wrap items-center gap-2">
              <Input
                value={rebuildPath}
                onChange={(e) => setRebuildPath(e.target.value)}
                placeholder="path, e.g. /about or /"
                className="h-9 max-w-sm"
              />
              <Button
                type="button"
                disabled={!rebuildPath.trim() || rebuilding}
                onClick={() => {
                  const p = rebuildPath.trim()
                  void rebuild({ variables: { paths: [p.startsWith('/') ? p : `/${p}`] } })
                  setRebuildPath('')
                }}
              >
                <RotateCcwIcon className="size-4" /> {rebuilding ? 'Rebuilding…' : 'Rebuild'}
              </Button>
            </div>
          </DetailPageSection>
        )}

        {canManage && (
          <DetailPageSection
            title="Queue rebuild (bulk)"
            description="Queue many paths for background rebuild - coalesced and paced by the worker (a few concurrent at a time), so large sets don't run as one long request. One path per line."
          >
            <div className="grid max-w-xl gap-2">
              <Textarea
                value={bulkPaths}
                onChange={(e) => setBulkPaths(e.target.value)}
                placeholder={'/about\n/products/123\n/landing/summer'}
                rows={4}
                className="font-mono text-xs"
              />
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  disabled={!bulkPaths.trim() || enqueuing}
                  onClick={async () => {
                    const paths = bulkPaths
                      .split('\n')
                      .map((s) => s.trim())
                      .filter(Boolean)
                    if (paths.length === 0) return
                    const res = await enqueue({ variables: { paths } })
                    setBulkPaths('')
                    setEnqueuedMsg(`Queued ${res.data?.enqueueRebuild ?? 0} path(s).`)
                  }}
                >
                  Queue rebuild
                </Button>
                {enqueuedMsg && <span className="text-xs text-muted-foreground">{enqueuedMsg}</span>}
              </div>
            </div>
          </DetailPageSection>
        )}

        <DetailPageSection
          title="Rebuild queue"
          description="Background rebuild tasks. The cache-rebuild worker drains them with bounded concurrency + retries; this refreshes automatically."
        >
          {!queue ? (
            <LoadingState />
          ) : (
            <div className="grid gap-3">
              <div className="flex flex-wrap gap-2 text-xs">
                <Badge variant="secondary">Pending {queue.pending}</Badge>
                <Badge variant="secondary">Running {queue.running}</Badge>
                <Badge variant="secondary">Done {queue.done}</Badge>
                <Badge variant={queue.failed > 0 ? 'destructive' : 'secondary'}>Failed {queue.failed}</Badge>
              </div>
              {queue.recent.length > 0 && (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b text-left text-xs text-muted-foreground">
                        <th className="py-1.5 pr-3">Path</th>
                        <th className="py-1.5 pr-3">Status</th>
                        <th className="py-1.5 pr-3">Attempts</th>
                        <th className="py-1.5">Error</th>
                      </tr>
                    </thead>
                    <tbody>
                      {queue.recent.map((t) => (
                        <tr key={t.id} className="border-b last:border-0">
                          <td className="py-1.5 pr-3"><code className="text-xs">{t.path}</code></td>
                          <td className="py-1.5 pr-3">
                            <Badge variant={t.status === 'failed' ? 'destructive' : 'secondary'}>{t.status}</Badge>
                          </td>
                          <td className="py-1.5 pr-3 text-muted-foreground">{t.attempts}</td>
                          <td className="py-1.5 text-muted-foreground">{t.lastError || '-'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}
        </DetailPageSection>

        <DetailPageSection
          title="Recent activity"
          description="The latest cache invalidations - manual ones and automatic ones (e.g. on publish)."
        >
          {logQ.loading ? (
            <LoadingState />
          ) : log.length === 0 ? (
            <p className="text-sm text-muted-foreground">No purges recorded yet.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-xs text-muted-foreground">
                    <th className="py-1.5 pr-3">When</th>
                    <th className="py-1.5 pr-3">Surface</th>
                    <th className="py-1.5 pr-3">Tags / paths</th>
                    <th className="py-1.5">Reason</th>
                  </tr>
                </thead>
                <tbody>
                  {log.map((e) => (
                    <tr key={e.id} className="border-b last:border-0">
                      <td className="whitespace-nowrap py-1.5 pr-3 text-muted-foreground">{formatDateTime(e.createdAt)}</td>
                      <td className="py-1.5 pr-3">{e.surface ? <Badge variant="secondary">{e.surface}</Badge> : '-'}</td>
                      <td className="py-1.5 pr-3">
                        <code className="text-xs">{[...e.tags, ...e.paths].join(', ') || '-'}</code>
                      </td>
                      <td className="py-1.5 text-muted-foreground">{e.reason || '-'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </DetailPageSection>
      </DetailPageBody>
    </DetailPageLayout>
  )
}
