import { useCallback, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { DatabaseIcon } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/ui/loading-state'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { DataResourcesDocument } from './Data.gql'
import { ExportPanel } from './ExportPanel'
import { ImportPanel } from './ImportPanel'
import { JobHistory } from './JobHistory'

// The one place bulk data moves in or out. Everything on this page is rendered from the backend's resource
// registry, so a newly registered resource shows up here with its columns and options and no frontend change
export function DataHub() {
  const [params, setParams] = useSearchParams()
  const { data, loading, error } = useQuery(DataResourcesDocument, {
    fetchPolicy: 'cache-and-network',
  })

  const resources = useMemo(() => data?.dataResources ?? [], [data])
  const formats = useMemo(() => data?.dataFormats ?? [], [data])
  const selectedSlug = params.get('resource') ?? resources[0]?.slug ?? ''
  const resource = resources.find((r) => r.slug === selectedSlug) ?? resources[0]

  const showAll = params.get('all') === '1'
  // Bumped when a run is queued - the job list watches it, because a fresh job is invisible to a query that
  // only fetches on mount and only polls once it has already seen something running
  const [jobsKey, setJobsKey] = useState(0)
  const reloadJobs = useCallback(() => setJobsKey((n) => n + 1), [])

  const setParam = useCallback(
    (key: string, value: string) => {
      const next = new URLSearchParams(params)
      next.set(key, value)
      if (value === '') next.delete(key)
      setParams(next, { replace: true })
    },
    [params, setParams],
  )


  const grouped = useMemo(() => {
    const byGroup = new Map<string, typeof resources>()
    for (const r of resources) {
      byGroup.set(r.group, [...(byGroup.get(r.group) ?? []), r])
    }
    return [...byGroup.entries()]
  }, [resources])

  if (loading && resources.length === 0) return <LoadingState />

  if (error) {
    return (
      <Alert variant="destructive" className="m-6">
        <AlertTitle>Could not load the transfer surface</AlertTitle>
        <AlertDescription>{error.message}</AlertDescription>
      </Alert>
    )
  }

  if (resources.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 py-20 text-center">
        <DatabaseIcon className="size-8 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Nothing here yet - your role does not grant access to any importable or exportable data.
        </p>
      </div>
    )
  }

  return (
    <DetailPageLayout
      toc={false}
      leftSidebar={
        <nav className="hidden w-60 shrink-0 overflow-y-auto border-r py-4 lg:block">
          {grouped.map(([group, items]) => (
            <div key={group} className="mb-4">
              <p className="px-4 pb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                {group}
              </p>
              <ul>
                {items.map((r) => (
                  <li key={r.slug}>
                    <button
                      type="button"
                      onClick={() => setParam('resource', r.slug)}
                      className={cn(
                        'flex w-full items-center justify-between gap-2 border-l-2 px-4 py-1.5 text-left text-sm transition-colors',
                        r.slug === resource?.slug
                          ? 'border-l-primary bg-muted/60 font-medium'
                          : 'border-l-transparent text-muted-foreground hover:bg-muted/40 hover:text-foreground',
                      )}
                    >
                      <span className="truncate">{r.label}</span>
                      {!r.canImport ? (
                        <span className="shrink-0 text-[10px] uppercase text-muted-foreground">
                          out
                        </span>
                      ) : null}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </nav>
      }
    >
      <DetailPageHeader
        backTo={null}
        title={resource?.label ?? 'Import & export'}
        badges={
          resource ? (
            <>
              <Badge variant="secondary">{resource.group}</Badge>
              {!resource.canImport ? <Badge variant="outline">export only</Badge> : null}
            </>
          ) : null
        }
        meta={<span>Bulk import and export - every resource, one place</span>}
      />

      <DetailPageBody>
        {/* The picker moves inline below the lg breakpoint, where the sidebar is hidden */}
        <DetailPageSection className="lg:hidden">
          <div className="flex flex-wrap gap-1.5">
            {resources.map((r) => (
              <button
                key={r.slug}
                type="button"
                onClick={() => setParam('resource', r.slug)}
                className={cn(
                  'rounded-md border px-2.5 py-1 text-xs transition-colors',
                  r.slug === resource?.slug
                    ? 'border-primary bg-muted font-medium'
                    : 'text-muted-foreground hover:bg-muted/50',
                )}
              >
                {r.label}
              </button>
            ))}
          </div>
        </DetailPageSection>

        {/* Both directions are on the page at once - a resource offers at most two of them and each
            panel is short, so a switch only hid one behind a click */}
        {resource ? (
          <>
            {resource.canExport ? (
              <DetailPageSection title="Export">
                <ExportPanel resource={resource} formats={formats} onQueued={reloadJobs} />
              </DetailPageSection>
            ) : null}

            {resource.canImport ? (
              <DetailPageSection title="Import">
                <ImportPanel resource={resource} formats={formats} onQueued={reloadJobs} />
              </DetailPageSection>
            ) : null}
          </>
        ) : null}

        <DetailPageSection
          title="Recent runs"
          description={`Everything moved in or out of ${resource?.label.toLowerCase() ?? 'this resource'}. Files stay downloadable until their retention window closes.`}
          trailing={
            <Button
              variant={showAll ? 'secondary' : 'outline'}
              size="sm"
              onClick={() => setParam('all', showAll ? '' : '1')}
            >
              {showAll ? 'This resource only' : 'All resources'}
            </Button>
          }
        >
          <JobHistory resourceSlug={showAll ? undefined : resource?.slug} reloadKey={jobsKey} />
        </DetailPageSection>
      </DetailPageBody>
    </DetailPageLayout>
  )
}
