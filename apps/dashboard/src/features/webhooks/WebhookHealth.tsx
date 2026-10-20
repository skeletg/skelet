import { useQuery } from '@apollo/client/react'
import {
  ActivityIcon,
  CheckCircle2Icon,
  TimerIcon,
  XCircleIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { WebhookDeliveryStatsDocument } from './Webhooks.gql'
import { timeAgo } from '@/lib/format'
import { useRefetchWhenShown, useVisiblePollInterval } from '@/lib/visible-poll'

// Traffic-light class for the success-rate figure - Only meaningful once at
// least one delivery has been recorded (callers gate on total > 0)
function rateClass(rate: number): string {
  if (rate >= 0.95) return 'text-success'
  if (rate >= 0.8) return 'text-warning'
  return 'text-destructive'
}

// WebhookHealth renders the aggregate delivery scorecard for one webhook: success rate, attempt counts, average response time, and the last
// delivery / success / failure timestamps - Polls so it stays fresh next to the (also-polling) delivery log
export function WebhookHealth({ webhookId }: { webhookId: string }) {
  const { data, loading, error, refetch } = useQuery(WebhookDeliveryStatsDocument, {
    variables: { webhookId },
    pollInterval: useVisiblePollInterval(15000),
  })
  useRefetchWhenShown(refetch)

  const stats = data?.webhookDeliveryStats

  if (loading && !stats) {
    return <p className="text-sm text-muted-foreground">Loading delivery health…</p>
  }
  if (error) {
    return (
      <p className="text-sm text-destructive">Couldn't load delivery health: {error.message}</p>
    )
  }
  if (!stats) return null

  if (stats.total === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No deliveries recorded yet. Attempts appear here once this webhook fires.
      </p>
    )
  }

  const ratePct = `${(stats.successRate * 100).toFixed(stats.successRate >= 0.995 || stats.successRate === 0 ? 0 : 1)}%`

  return (
    <div className="grid gap-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat
          icon={<CheckCircle2Icon className="size-4" />}
          label="Success rate"
          value={<span className={rateClass(stats.successRate)}>{ratePct}</span>}
          hint={`${stats.succeeded} of ${stats.total} attempts`}
        />
        <Stat
          icon={<ActivityIcon className="size-4" />}
          label="Delivered"
          value={stats.total.toLocaleString()}
          hint="attempts incl. retries"
        />
        <Stat
          icon={<XCircleIcon className="size-4" />}
          label="Failed"
          value={
            <span className={stats.failed > 0 ? 'text-destructive' : undefined}>
              {stats.failed.toLocaleString()}
            </span>
          }
          hint={stats.failed > 0 ? 'needs attention' : 'none'}
        />
        <Stat
          icon={<TimerIcon className="size-4" />}
          label="Avg response"
          value={`${stats.avgSuccessMs.toLocaleString()}ms`}
          hint="successful only"
        />
      </div>

      <dl className="divide-y rounded-md border text-sm">
        <TimeRow label="Last delivery" iso={stats.lastDeliveryAt} />
        <TimeRow label="Last success" iso={stats.lastSuccessAt} />
        <TimeRow label="Last failure" iso={stats.lastFailureAt} emphasizeError />
      </dl>
    </div>
  )
}

function Stat({
  icon,
  label,
  value,
  hint,
}: {
  icon: React.ReactNode
  label: string
  value: React.ReactNode
  hint?: string
}) {
  return (
    <div className="grid gap-1 rounded-md border p-3">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {icon}
        <span className="uppercase tracking-wide">{label}</span>
      </div>
      <span className="text-2xl font-semibold tabular-nums leading-none">{value}</span>
      {hint && <span className="text-[11px] text-muted-foreground">{hint}</span>}
    </div>
  )
}

function TimeRow({
  label,
  iso,
  emphasizeError,
}: {
  label: string
  iso?: string | null
  emphasizeError?: boolean
}) {
  return (
    <div className="grid grid-cols-[130px_1fr] items-baseline gap-3 px-3 py-2">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn('flex flex-wrap items-baseline gap-x-2', !iso && 'text-muted-foreground')}>
        {iso ? (
          <>
            <span className={cn('font-medium', emphasizeError && 'text-destructive')}>
              {timeAgo(iso)}
            </span>
            <span className="text-xs text-muted-foreground">
              {new Date(iso).toLocaleString()}
            </span>
          </>
        ) : (
          '—'
        )}
      </dd>
    </div>
  )
}
