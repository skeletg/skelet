import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { LensCustomerSessionsDocument, LensVisitorSessionsDocument } from './Lens.signals.gql'

// One row's worth of a recording, as returned by the LensSession* queries
export type SessionRow = {
  id: string
  anonId: string
  customerId?: string | null
  startedAt: string
  durationMs: number
  eventCount: number
  entryPage: string
  exitPage: string
  viewport: string
}

function fmtDur(ms: number): string {
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  return `${m}m ${String(s % 60).padStart(2, '0')}s`
}
function fmtWhen(v: string): string {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

// SessionRows is the shared presentational list of recordings - used by the
// channel Recordings browser and the per-profile "Session recordings" panels
// Each row links to the rrweb player. showVisitor adds the anon/customer tag
// (the Recordings browser wants it; a profile already knows who it is)
export function SessionRows({
  sessions,
  emptyText = 'No recordings yet.',
  showVisitor = false,
}: {
  sessions: SessionRow[]
  emptyText?: string
  showVisitor?: boolean
}) {
  if (!sessions.length) return <p className="text-sm text-muted-foreground">{emptyText}</p>
  return (
    <ul className="space-y-1.5">
      {sessions.map((s) => (
        <li key={s.id}>
          <Link
            to={`/lens/sessions/${s.id}`}
            className="flex items-center justify-between gap-3 rounded-lg border p-2.5 text-sm hover:bg-muted"
          >
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-medium tabular-nums">{fmtDur(s.durationMs)}</span>
                <span className="text-muted-foreground">·</span>
                <span className="truncate font-mono text-xs text-muted-foreground">
                  {s.entryPage || '/'}
                  {s.exitPage && s.exitPage !== s.entryPage ? ` → ${s.exitPage}` : ''}
                </span>
              </div>
              <div className="text-xs text-muted-foreground">
                {fmtWhen(s.startedAt)} · {s.eventCount.toLocaleString()} events · {s.viewport}
                {showVisitor ? ` · ${s.customerId ? 'customer' : 'anon ' + s.anonId.slice(0, 8)}` : ''}
              </div>
            </div>
            <span className="shrink-0 text-xs font-medium text-primary">▶ Replay</span>
          </Link>
        </li>
      ))}
    </ul>
  )
}

const RECORDINGS_DESC = 'Replays of this visitor’s sessions (rrweb). Inputs were masked at capture.'

// CustomerRecordingsSection is the "Session recordings" panel on the Synapse
// customer profile - the customer's sessions (by id + any identified anon ids)
// Renders nothing (no section) when there are no recordings, so the profile
// isn't cluttered for customers who were never sampled
export function CustomerRecordingsSection({ customerId }: { customerId: string }) {
  const { data, loading } = useQuery(LensCustomerSessionsDocument, {
    variables: { customerId, limit: 25 },
    skip: !customerId,
    fetchPolicy: 'cache-and-network',
  })
  const sessions = data?.lensCustomerSessions ?? []
  if (!loading && sessions.length === 0) return null
  return (
    <DetailPageSection title="Session recordings" description={RECORDINGS_DESC}>
      <SessionRows sessions={sessions} emptyText={loading ? 'Loading…' : 'No recordings.'} />
    </DetailPageSection>
  )
}

// VisitorRecordingsSection is the same panel for an anonymous visitor profile
export function VisitorRecordingsSection({ anonId }: { anonId: string }) {
  const { data, loading } = useQuery(LensVisitorSessionsDocument, {
    variables: { anonId, limit: 25 },
    skip: !anonId,
    fetchPolicy: 'cache-and-network',
  })
  const sessions = data?.lensVisitorSessions ?? []
  if (!loading && sessions.length === 0) return null
  return (
    <DetailPageSection title="Session recordings" description={RECORDINGS_DESC}>
      <SessionRows sessions={sessions} emptyText={loading ? 'Loading…' : 'No recordings.'} />
    </DetailPageSection>
  )
}
