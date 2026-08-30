import { useQuery } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { ScheduledJobsDocument } from '@/features/scheduler/Scheduler.gql'
import { WidgetSection } from './primitives'

const ROWS = 6

function when(iso: string | null | undefined): string {
  if (!iso) return '-'
  const mins = Math.round((new Date(iso).getTime() - Date.now()) / 60_000)
  if (Math.abs(mins) < 1) return 'now'
  if (mins > 0) return mins < 60 ? `in ${mins}m` : `in ${Math.round(mins / 60)}h`
  return -mins < 60 ? `${-mins}m ago` : `${Math.round(-mins / 60)}h ago`
}

// Failing jobs first, because a job that stopped running is silent by nature - that is the whole
// reason the notification scan exists
export function JobsWidget() {
  const { data, loading, error } = useQuery(ScheduledJobsDocument, { fetchPolicy: 'cache-and-network' })
  const jobs = data?.scheduledJobs ?? []
  const failing = jobs.filter((j) => j.enabled && j.lastStatus === 'error')
  const rest = jobs
    .filter((j) => j.enabled && j.lastStatus !== 'error')
    .sort((a, b) => new Date(a.nextRunAt ?? 0).getTime() - new Date(b.nextRunAt ?? 0).getTime())
  const rows = [...failing, ...rest].slice(0, ROWS)

  return (
    <WidgetSection
      title="Scheduled jobs"
      description={
        failing.length > 0
          ? `${failing.length} ${failing.length === 1 ? 'job' : 'jobs'} failed on its last run.`
          : 'Everything is running on schedule.'
      }
      to="/scheduled-jobs"
      loading={loading && jobs.length === 0}
      error={error ?? null}
      empty={!loading && rows.length === 0}
    >
      <div className="divide-y rounded-md border">
        {rows.map((job) => {
          const bad = job.lastStatus === 'error'
          return (
            <Link
              key={job.id}
              to={`/scheduled-jobs/${job.id}`}
              className="flex items-center gap-3 px-3 py-2 transition hover:bg-muted/40"
            >
              <span className={cn('h-8 w-0.5 shrink-0 rounded-full', bad ? 'bg-destructive' : 'bg-primary/40')} />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm">{job.name}</span>
                <span className="block truncate text-xs text-muted-foreground">
                  {bad ? (job.lastError || 'last run failed') : job.schedule}
                </span>
              </span>
              <span className="shrink-0 text-right text-xs text-muted-foreground">
                <span className="block">{bad ? `${job.consecutiveFailures}x failed` : 'next'}</span>
                <span className="block tabular-nums">{when(bad ? job.lastRunAt : job.nextRunAt)}</span>
              </span>
            </Link>
          )
        })}
      </div>
    </WidgetSection>
  )
}
