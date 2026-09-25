import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { DataJob } from './jobs'
import { isRunning } from './jobs'

// Semantic state, so these are fixed colours rather than the user-customizable theme tokens
export function JobStatusBadge({ status, dryRun }: { status: string; dryRun?: boolean }) {
  const label = dryRun && status === 'succeeded' ? 'validated' : status.replace('_', ' ')
  switch (status) {
    case 'succeeded':
      return (
        <Badge className="bg-success-soft text-success">
          {label}
        </Badge>
      )
    case 'failed':
      return <Badge variant="destructive">{label}</Badge>
    case 'cancelled':
      return <Badge variant="outline">{label}</Badge>
    case 'running':
      return (
        <Badge className="bg-info-soft text-info">
          {label}
        </Badge>
      )
    default:
      return <Badge variant="secondary">{label}</Badge>
  }
}

// An export doesn't know its total until it finishes, so it shows a row count rather than a fake percentage
export function JobProgress({ job, className }: { job: DataJob; className?: string }) {
  if (!isRunning(job)) return null
  const known = job.totalRows > 0
  const pct = known ? Math.min(100, Math.round((job.processedRows / job.totalRows) * 100)) : null
  return (
    <div className={cn('grid gap-1.5', className)}>
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
        <div
          className={cn('h-full bg-info transition-[width] duration-300', pct === null && 'animate-pulse')}
          style={{ width: pct === null ? '100%' : `${pct}%` }}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {job.status === 'queued'
          ? 'Waiting for a runner'
          : known
            ? `${job.processedRows.toLocaleString()} of ${job.totalRows.toLocaleString()} rows`
            : `${job.processedRows.toLocaleString()} rows so far`}
      </p>
    </div>
  )
}

export function JobCounts({ job }: { job: DataJob }) {
  if (job.direction === 'export') {
    return <span>{job.totalRows.toLocaleString()} rows</span>
  }
  const parts: string[] = []
  if (job.createdRows) parts.push(`${job.createdRows.toLocaleString()} created`)
  if (job.updatedRows) parts.push(`${job.updatedRows.toLocaleString()} updated`)
  if (job.skippedRows) parts.push(`${job.skippedRows.toLocaleString()} skipped`)
  if (job.failedRows) parts.push(`${job.failedRows.toLocaleString()} failed`)
  return <span>{parts.length ? parts.join(', ') : '-'}</span>
}
