import { useQuery } from '@apollo/client/react'
import { AuditLogDocument } from '@/features/audit/Audit.gql'
import { WidgetSection } from './primitives'

const ROWS = 8

// `detail` is the redacted operation variables as free-form JSON, so pull out the couple of scalar
// fields that identify what was touched rather than dumping the blob. The backend redacts secret
// VALUES, but a row reading "token …" is noise either way, so those keys are dropped too
const SECRETISH = /token|password|secret|key|authorization/i

function summarize(detail: unknown): string {
  if (!detail || typeof detail !== 'object') return ''
  return Object.entries(detail as Record<string, unknown>)
    .filter(([k, v]) => !SECRETISH.test(k) && (typeof v === 'string' || typeof v === 'number'))
    .slice(0, 2)
    .map(([k, v]) => `${k} ${v}`)
    .join(' · ')
}

export function AuditWidget() {
  const { data, loading, error } = useQuery(AuditLogDocument, {
    variables: { limit: ROWS },
    fetchPolicy: 'cache-and-network',
  })
  const entries = data?.auditLog.entries ?? []

  return (
    <WidgetSection
      title="Recent admin activity"
      description="Who changed what, newest first."
      to="/audit-log"
      loading={loading && entries.length === 0}
      error={error ?? null}
      empty={!loading && entries.length === 0}
    >
      <div className="divide-y rounded-md border">
        {entries.map((e) => (
          <div key={e.id} className="flex items-center gap-3 px-3 py-2">
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm">
                <span className="font-mono text-xs">{e.action}</span>
                <span className="ml-2 text-muted-foreground">{summarize(e.detail)}</span>
              </span>
              <span className="block truncate text-xs text-muted-foreground">{e.adminEmail}</span>
            </span>
            {!e.success && <span className="shrink-0 text-xs text-destructive">failed</span>}
            <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
              {new Date(e.createdAt).toLocaleTimeString(undefined, { timeStyle: 'short' })}
            </span>
          </div>
        ))}
      </div>
    </WidgetSection>
  )
}
