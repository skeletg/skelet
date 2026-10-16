import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { AlertTriangleIcon, DownloadIcon, ListXIcon, XIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/ui/loading-state'
import { cn } from '@/lib/utils'
import { CancelDataJobDocument, DataJobsDocument } from './Data.gql'
import { downloadJobArtifact } from './api'
import { JobErrorsSheet } from './JobErrorsSheet'
import { JobCounts, JobProgress, JobStatusBadge } from './JobStatus'
import { formatTimestamp } from '@/lib/format'
import { isRunning, type DataJob } from './jobs'

// While anything is in flight the list re-reads every second - a finished board stops polling entirely,
// so an idle hub costs nothing
const ACTIVE_POLL_MS = 1000

export function JobHistory({
  resourceSlug,
  limit = 25,
  reloadKey = 0,
}: {
  resourceSlug?: string
  limit?: number
  // Bumped by the page when a run is queued - a brand-new job is invisible to a query that only fetches on
  // mount and only polls once it has already seen something running, so it needs an explicit nudge
  reloadKey?: number
}) {
  const [errorsFor, setErrorsFor] = useState<DataJob | null>(null)
  const [running, setRunning] = useState(0)
  // pollInterval rather than start/stopPolling, so the board speeds up while work is in flight and goes
  // completely quiet once nothing is - useQuery has to be called before its own result can be counted, so
  // this reads the previous render's count and adjusts during render (never in an effect)
  const { data, loading, refetch } = useQuery(DataJobsDocument, {
    variables: { resources: resourceSlug ? [resourceSlug] : null, limit },
    fetchPolicy: 'cache-and-network',
    pollInterval: running > 0 ? ACTIVE_POLL_MS : 0,
  })
  const [cancel] = useMutation(CancelDataJobDocument)

  useEffect(() => {
    if (reloadKey > 0) void refetch()
  }, [reloadKey, refetch])

  const jobs = useMemo(() => data?.dataJobs.items ?? [], [data])
  const active = jobs.filter(isRunning).length
  if (active !== running) setRunning(active)

  if (loading && jobs.length === 0) return <LoadingState compact />

  if (jobs.length === 0) {
    return (
      <div className="flex flex-col items-center gap-2 rounded-md border border-dashed py-10 text-center">
        <ListXIcon className="size-6 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">Nothing has been imported or exported yet.</p>
      </div>
    )
  }

  return (
    <>
      <div className="overflow-hidden rounded-md border divide-y">
        {jobs.map((job) => (
          <div
            key={job.id}
            className={cn(
              'grid gap-3 border-l-2 p-3 sm:grid-cols-[1fr_auto] sm:items-center',
              job.status === 'failed'
                ? 'border-l-destructive'
                : job.status === 'succeeded'
                  ? 'border-l-emerald-500'
                  : 'border-l-transparent',
            )}
          >
            <div className="grid min-w-0 gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{job.resourceLabel}</span>
                <span className="text-xs uppercase tracking-wide text-muted-foreground">
                  {job.direction}
                </span>
                <JobStatusBadge status={job.status} dryRun={job.dryRun} />
                {job.dryRun && job.status !== 'succeeded' ? (
                  <span className="text-xs text-muted-foreground">validate only</span>
                ) : null}
              </div>
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                <span>{formatTimestamp(job.createdAt)}</span>
                <JobCounts job={job} />
                {job.filename ? <span className="truncate font-mono">{job.filename}</span> : null}
              </div>
              <JobProgress job={job} className="max-w-sm" />
              {job.warnings.map((w) => (
                <p key={w} className="flex items-start gap-1.5 text-xs text-warning">
                  <AlertTriangleIcon className="mt-0.5 size-3 shrink-0" />
                  {w}
                </p>
              ))}
              {job.errorSummary ? (
                <p className="text-xs text-destructive">{job.errorSummary}</p>
              ) : null}
            </div>

            <div className="flex shrink-0 flex-wrap items-center gap-2">
              {job.failedRows > 0 ? (
                <Button variant="outline" size="sm" onClick={() => setErrorsFor(job)}>
                  {job.failedRows.toLocaleString()} failed rows
                </Button>
              ) : null}
              {job.hasArtifact && job.direction === 'export' && job.status === 'succeeded' ? (
                <Button size="sm" onClick={() => downloadJobArtifact(job.id, job.filename)}>
                  <DownloadIcon className="mr-2 size-4" />
                  Download
                </Button>
              ) : null}
              {job.hasArtifact && job.direction === 'import' ? (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => downloadJobArtifact(job.id, job.filename)}
                >
                  <DownloadIcon className="mr-2 size-4" />
                  Source file
                </Button>
              ) : null}
              {isRunning(job) ? (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={async () => {
                    await cancel({ variables: { id: job.id } })
                    await refetch()
                  }}
                >
                  <XIcon className="mr-2 size-4" />
                  Cancel
                </Button>
              ) : null}
            </div>
          </div>
        ))}
      </div>
      {errorsFor ? <JobErrorsSheet job={errorsFor} onClose={() => setErrorsFor(null)} /> : null}
    </>
  )
}
