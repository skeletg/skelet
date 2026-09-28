import { useQuery } from '@apollo/client/react'
import { cn } from '@/lib/utils'
import { SystemHealthDocument } from '@/features/system-health/SystemHealth.gql'
import { WidgetSection } from './primitives'

const TONE: Record<string, string> = {
  OK: 'bg-success-soft text-success',
  WARN: 'bg-warning-soft text-warning',
  BAD: 'bg-destructive-soft text-destructive',
  OFF: 'bg-muted text-muted-foreground',
}

const HEADLINE: Record<string, string> = {
  OK: 'Everything reporting healthy.',
  WARN: 'Something wants a look.',
  BAD: 'A subsystem is failing.',
  OFF: 'Nothing configured to check.',
}

// A rollup, not the full page: a chip per group plus the checks that are actually unhappy, since
// most of these failures are silent and only visible in container logs otherwise
export function SystemHealthWidget() {
  const { data, loading, error } = useQuery(SystemHealthDocument, { fetchPolicy: 'cache-and-network' })
  const health = data?.systemHealth
  const groups = health?.groups ?? []
  const unhappy = groups.flatMap((g) =>
    g.checks.filter((c) => c.status === 'WARN' || c.status === 'BAD').map((c) => ({ ...c, group: g.label })),
  )

  return (
    <WidgetSection
      title="System health"
      description={health ? HEADLINE[health.status] : undefined}
      to="/system-health"
      toLabel="All checks"
      loading={loading && !health}
      error={error ?? null}
      empty={!loading && !health}
    >
      <div className="grid gap-3">
        <div className="flex flex-wrap gap-1.5">
          {groups.map((g) => {
            const worst = g.checks.some((c) => c.status === 'BAD')
              ? 'BAD'
              : g.checks.some((c) => c.status === 'WARN')
                ? 'WARN'
                : g.checks.every((c) => c.status === 'OFF')
                  ? 'OFF'
                  : 'OK'
            return (
              <span key={g.key} className={cn('rounded-md px-2 py-1 text-xs', TONE[worst])}>
                {g.label}
              </span>
            )
          })}
        </div>
        {unhappy.length > 0 && (
          <div className="divide-y rounded-md border">
            {unhappy.map((c) => (
              <div key={c.key} className="flex items-start gap-3 px-3 py-2">
                <span className={cn('mt-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium', TONE[c.status])}>
                  {c.status}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm">{c.label}</span>
                  <span className="block text-xs text-muted-foreground">{c.detail}</span>
                  {c.hint && <span className="mt-0.5 block text-xs text-primary">{c.hint}</span>}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </WidgetSection>
  )
}
