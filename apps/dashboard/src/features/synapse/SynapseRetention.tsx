import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { useConfigScope } from '@/lib/config-scope'
import { eventLabel } from './eventLabel'
import { SynapseOverviewDocument, SynapseRetentionDocument } from './Synapse.gql'

type Unit = 'day' | 'week'
type Bucket = { period: number; count: number; eligible: number }
const nfmt = (n: number) => n.toLocaleString()
const pctOf = (b?: Bucket | null): number | null => (b && b.eligible > 0 ? Math.round((b.count / b.eligible) * 100) : null)

export function SynapseRetention() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [startEvent, setStart] = useState('page_view')
  const [returnEvent, setReturn] = useState('page_view')
  const [unit, setUnit] = useState<Unit>('day')
  const periods = unit === 'day' ? 14 : 8

  const { data: ov, error: ovError } = useQuery(SynapseOverviewDocument, {
    variables: { channelId, sinceDays: 30 },
    skip: !isChannel,
    fetchPolicy: 'cache-and-network',
  })
  const events = (ov?.synapseOverview.topEvents ?? []).map((e) => e.name)

  const { data, loading, error: retError } = useQuery(SynapseRetentionDocument, {
    variables: { channelId, startEvent, returnEvent, sinceDays: 60, periods, unit },
    skip: !isChannel || !startEvent || !returnEvent,
    fetchPolicy: 'cache-and-network',
  })
  const ret = data?.synapseRetention
  const error = ovError ?? retError
  const unitLabel = unit === 'day' ? 'Day' : 'Wk'
  const fmtCohort = (v: string) => {
    const d = new Date(v)
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  }

  // Headline figures off the blended curve
  const p1 = pctOf(ret?.buckets[1])
  const lastMature = [...(ret?.buckets ?? [])].reverse().find((b) => b.eligible > 0)
  const lastPct = pctOf(lastMature)

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Retention"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Of the people who did the start event, how many came back and did the return event in each later period -
            broken out by when they started, so you can see if newer cohorts hold up better. Cohort window: last 60 days.
          </span>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load retention</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Retention is per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection
            title="Retention"
            description="Each cell is the share of that cohort who returned in that period, out of the cohort members old enough to have reached it. The top row blends all cohorts."
            trailing={
              <div className="flex flex-wrap items-end gap-3 text-sm">
                <Field label="Start event">
                  <EventSelect value={startEvent} onChange={setStart} events={events} />
                </Field>
                <Field label="Came back & did">
                  <EventSelect value={returnEvent} onChange={setReturn} events={events} />
                </Field>
                <Field label="By">
                  <div className="inline-flex rounded-md border p-0.5">
                    {(['day', 'week'] as Unit[]).map((u) => (
                      <button
                        key={u}
                        type="button"
                        onClick={() => setUnit(u)}
                        className={cn('rounded px-2.5 py-1 text-xs capitalize', unit === u ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground')}
                      >
                        {u}
                      </button>
                    ))}
                  </div>
                </Field>
              </div>
            }
          >
            {!ret ? (
              <p className="py-6 text-sm text-muted-foreground">{loading ? 'Computing…' : 'No data.'}</p>
            ) : ret.cohort === 0 ? (
              <p className="py-6 text-sm text-muted-foreground">No one did “{eventLabel(startEvent)}” in the last 60 days - pick another start event.</p>
            ) : (
              <div className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-3">
                  <Tile label="Cohort" value={nfmt(ret.cohort)} sub={`did ${eventLabel(startEvent)}`} />
                  <Tile label={`${unitLabel} 1 retention`} value={p1 === null ? '-' : `${p1}%`} sub={`came back & did ${eventLabel(returnEvent)}`} />
                  <Tile label={lastMature ? `${unitLabel} ${lastMature.period} retention` : 'Latest'} value={lastPct === null ? '-' : `${lastPct}%`} sub="most recent mature period" />
                </div>

                <div className="overflow-x-auto rounded border">
                  <table className="border-separate border-spacing-0 text-xs">
                    <thead>
                      <tr className="text-muted-foreground">
                        <th className="sticky left-0 z-10 bg-background px-3 py-2 text-left font-medium">Cohort</th>
                        <th className="px-2 py-2 text-right font-medium">People</th>
                        {Array.from({ length: periods }, (_, p) => (
                          <th key={p} className="px-1 py-2 text-center font-medium">
                            {unitLabel} {p}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      <CohortRow label="All cohorts" size={ret.cohort} buckets={ret.buckets} emphasize />
                      {ret.cohorts.map((c) => (
                        <CohortRow key={c.startAt} label={fmtCohort(c.startAt)} size={c.size} buckets={c.buckets} />
                      ))}
                    </tbody>
                  </table>
                </div>
                <p className="text-xs text-muted-foreground">
                  Blank cells are periods a cohort is too new to have reached. Each %’s denominator is only the
                  cohort members mature enough for that period, so young cohorts don’t deflate later columns.
                </p>
              </div>
            )}
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function CohortRow({ label, size, buckets, emphasize }: { label: string; size: number; buckets: Bucket[]; emphasize?: boolean }) {
  return (
    <tr className={cn('border-t', emphasize && 'bg-muted/40 font-medium')}>
      <td className={cn('sticky left-0 z-10 whitespace-nowrap px-3 py-1.5', emphasize ? 'bg-muted/40 font-medium' : 'bg-background')}>{label}</td>
      <td className="px-2 py-1.5 text-right tabular-nums text-muted-foreground">{nfmt(size)}</td>
      {buckets.map((b) => {
        const p = pctOf(b)
        return (
          <td key={b.period} className="px-0.5 py-1">
            {p === null ? (
              <div className="mx-auto grid h-8 w-12 place-items-center rounded bg-muted/20 text-muted-foreground/30">·</div>
            ) : (
              <div className="relative mx-auto grid h-8 w-12 place-items-center overflow-hidden rounded" title={`${nfmt(b.count)} of ${nfmt(b.eligible)}`}>
                <div className="absolute inset-0 bg-primary" style={{ opacity: Math.max(0.06, p / 100) }} aria-hidden />
                <span className={cn('relative text-[11px] tabular-nums', p >= 55 ? 'text-primary-foreground' : 'text-foreground')}>{p}%</span>
              </div>
            )}
          </td>
        )
      })}
    </tr>
  )
}

function Tile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded border p-4">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
      {sub && <p className="mt-0.5 truncate text-xs text-muted-foreground">{sub}</p>}
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-xs uppercase tracking-wider text-muted-foreground">{label}</span>
      {children}
    </label>
  )
}

function EventSelect({ value, onChange, events }: { value: string; onChange: (v: string) => void; events: string[] }) {
  const options = events.includes(value) ? events : [value, ...events]
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} className="rounded border bg-background px-2 py-1.5 text-sm">
      {options.map((e) => (
        <option key={e} value={e}>
          {eventLabel(e)}
        </option>
      ))}
    </select>
  )
}
