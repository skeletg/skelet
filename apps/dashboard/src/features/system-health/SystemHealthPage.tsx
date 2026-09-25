import { useQuery } from '@apollo/client/react'
import {
  AlertTriangleIcon,
  CheckCircle2Icon,
  CircleSlashIcon,
  RefreshCwIcon,
} from 'lucide-react'

import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/ui/loading-state'
import { cn } from '@/lib/utils'
import type { HealthStatus } from '@/graphql/schema.types'
import { SystemHealthDocument } from './SystemHealth.gql'
import { type Check, meaningful, useJustChanged } from './refreshCue'

// Groups and rows are whatever the backend registered, so nothing here is keyed to a specific subsystem - a new one appears with no change
// to this file Semantic colours are spelled out rather than taken from primary/accent, which are theme tokens an operator can recolour
const TONE: Record<HealthStatus, string> = {
  OK: 'text-success',
  WARN: 'text-warning',
  BAD: 'text-destructive',
  OFF: 'text-muted-foreground',
}

const HEADLINE: Record<HealthStatus, string> = {
  OK: 'All systems normal',
  WARN: 'Needs attention',
  BAD: 'Something is broken',
  OFF: 'Nothing configured',
}

function StatusIcon({ status, className }: { status: HealthStatus; className?: string }) {
  const cls = cn('size-4 shrink-0', TONE[status], className)
  if (status === 'OFF') return <CircleSlashIcon className={cls} />
  if (status === 'OK') return <CheckCircle2Icon className={cls} />
  return <AlertTriangleIcon className={cls} />
}

// OFF never escalates, mirroring the backend - a subsystem nobody deployed is not a fault
function rollUp(checks: Check[]): HealthStatus {
  if (checks.some((c) => c.status === 'BAD')) return 'BAD'
  if (checks.some((c) => c.status === 'WARN')) return 'WARN'
  if (checks.some((c) => c.status === 'OK')) return 'OK'
  return 'OFF'
}

function countBy(checks: Check[], status: HealthStatus): number {
  return checks.filter((c) => c.status === status).length
}

function checkedAgo(iso?: string): string {
  if (!iso) return ''
  const secs = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000)
  return secs < 90 ? `${Math.round(secs)}s ago` : `${Math.round(secs / 60)}m ago`
}

export function SystemHealthPage() {
  const { data, loading, error, refetch } = useQuery(SystemHealthDocument, {
    fetchPolicy: 'cache-and-network',
    pollInterval: 5_000,
  })
  const health = data?.systemHealth
  const groups = health?.groups ?? []
  const all: Check[] = groups.flatMap((g) => g.checks)

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="System health"
        meta={
          <span>
            Datastores, search, background work and delivery across every process
            {all.length > 0 ? ` - ${all.length} checks in ${groups.length} groups` : ''}
            {health ? (
              <span key={health.checkedAt} data-health-tick>
                , checked {checkedAgo(health.checkedAt)}
              </span>
            ) : null}
          </span>
        }
        actions={
          <div className="flex items-center gap-2">
            {health ? (
              <Badge variant="outline" className={cn('gap-1.5', TONE[health.status])}>
                <StatusIcon status={health.status} className="size-3.5" />
                {HEADLINE[health.status]}
              </Badge>
            ) : null}
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={loading}>
              <RefreshCwIcon className={cn('size-4', loading && 'animate-spin')} />
              Refresh
            </Button>
          </div>
        }
      />
      <DetailPageBody>
        {error ? (
          <div className="px-6 pt-4">
            <Alert variant="destructive">
              <AlertTriangleIcon />
              <AlertTitle>Couldn't read system health</AlertTitle>
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          </div>
        ) : null}

        {!health && loading ? <LoadingState label="Probing subsystems…" /> : null}

        {health ? (
          <>
            <DetailPageSection>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                <Stat
                  icon={<CheckCircle2Icon className="size-4" />}
                  label="Passing"
                  value={countBy(all, 'OK')}
                  tone={TONE.OK}
                  hint="reporting healthy"
                />
                <Stat
                  icon={<AlertTriangleIcon className="size-4" />}
                  label="Attention"
                  value={countBy(all, 'WARN')}
                  tone={countBy(all, 'WARN') > 0 ? TONE.WARN : undefined}
                  hint={countBy(all, 'WARN') > 0 ? 'degraded, still working' : 'none'}
                />
                <Stat
                  icon={<AlertTriangleIcon className="size-4" />}
                  label="Broken"
                  value={countBy(all, 'BAD')}
                  tone={countBy(all, 'BAD') > 0 ? TONE.BAD : undefined}
                  hint={countBy(all, 'BAD') > 0 ? 'needs a fix now' : 'none'}
                />
                <Stat
                  icon={<CircleSlashIcon className="size-4" />}
                  label="Not configured"
                  value={countBy(all, 'OFF')}
                  hint="optional, switched off"
                />
              </div>
            </DetailPageSection>

            {groups.map((group) => (
              <DetailPageSection
                key={group.key}
                title={group.label}
                trailing={
                  <span className="flex items-center gap-2 pt-1">
                    <StatusIcon status={rollUp(group.checks)} className="size-3.5" />
                    <span className="text-xs text-muted-foreground tabular-nums">
                      {groupSummary(group.checks)}
                    </span>
                  </span>
                }
              >
                <div className="divide-y rounded-md border">
                  {group.checks.map((check) => (
                    <CheckRow key={check.key} check={check} />
                  ))}
                </div>
              </DetailPageSection>
            ))}
          </>
        ) : null}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// Reads as "2 broken, 5 passing" - the roll-up a group's rows would make you count
function groupSummary(checks: Check[]): string {
  const parts: string[] = []
  const bad = countBy(checks, 'BAD')
  const warn = countBy(checks, 'WARN')
  const ok = countBy(checks, 'OK')
  const off = countBy(checks, 'OFF')
  if (bad) parts.push(`${bad} broken`)
  if (warn) parts.push(`${warn} to check`)
  if (ok) parts.push(`${ok} passing`)
  if (off && !parts.length) parts.push(`${off} not configured`)
  else if (off) parts.push(`${off} off`)
  return parts.join(', ')
}

function CheckRow({ check }: { check: Check }) {
  const changed = useJustChanged(meaningful(check))
  return (
    <div className="flex items-start gap-3 px-4 py-3" data-health-changed={changed}>
      <StatusIcon status={check.status} className="mt-0.5" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span className="text-sm font-medium">{check.label}</span>
          <span className="text-sm text-muted-foreground tabular-nums">{check.detail}</span>
        </div>
        {check.hint ? <p className={cn('mt-1 text-xs', TONE[check.status])}>{check.hint}</p> : null}
      </div>
    </div>
  )
}

function Stat({
  icon,
  label,
  value,
  hint,
  tone,
}: {
  icon: React.ReactNode
  label: string
  value: number
  hint?: string
  tone?: string
}) {
  const changed = useJustChanged(String(value))
  return (
    <div className="grid gap-1 rounded-md border p-3" data-health-changed={changed}>
      <div className={cn('flex items-center gap-1.5 text-xs', tone ?? 'text-muted-foreground')}>
        {icon}
        <span className="uppercase tracking-wide">{label}</span>
      </div>
      <span className={cn('text-2xl font-semibold tabular-nums leading-none', tone)}>{value}</span>
      {hint ? <span className="text-[11px] text-muted-foreground">{hint}</span> : null}
    </div>
  )
}

export default SystemHealthPage
