import { useQuery } from '@apollo/client/react'
import { cn } from '@/lib/utils'
import { formatNumber } from '@/lib/format'
import { eventLabel } from '@/features/synapse/eventLabel'
import { SynapseRetentionDocument, SynapseStickinessDocument } from '@/features/synapse/Synapse.gql'
import { describeConditions, readyConditions, type ControlsProps, type Kind, type ReportDefinition, type ResultProps } from './definition'
import { ConditionsEditor } from './conditions'
import { Bars, EventSelect, Field, Headline, Note, State, StatTile, Toggle, pct, useTopEvents } from './ui'

type Bucket = { period: number; count: number; eligible: number }
const pctOf = (b?: Bucket | null): number | null => (b && b.eligible > 0 ? Math.round((b.count / b.eligible) * 100) : null)
// Below this many people a cohort's percentages swing on one person, so its cells are faded and say so
export const SMALL_COHORT = 20

// Common return cadences - a store picks what "coming back" means for its catalog
const WINDOWS = [7, 14, 30, 60, 90, 180, 365]
const windowLabel = (d: number) => (d % 365 === 0 ? `${d / 365}y` : d % 30 === 0 ? `${d / 30}mo` : d % 7 === 0 ? `${d / 7}wk` : `${d}d`)
const fmtDay = (v: string) => {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

// What an unset unit reads as, and how many periods a unit reads
const UNIT = 'week'
const periodsFor = (unit: string) => (unit === 'day' ? 14 : 8)

// One builder answers "do people come back?" two ways; switching view carries the event across
export function switchRetentionView(d: ReportDefinition, view: 'STICKINESS' | 'RETENTION'): ReportDefinition {
  if (view === d.kind) return d
  const { sinceDays } = d
  if (view === 'STICKINESS') return { kind: 'STICKINESS', event: d.returnEvent || undefined, where: d.returnWhere, sinceDays }
  const ev = d.event || retention.start.startEvent
  return { ...retention.start, startEvent: ev, returnEvent: ev, startWhere: d.where, returnWhere: d.where, sinceDays }
}

function RetentionControls({ channelId, value: d, onChange }: ControlsProps) {
  const events = useTopEvents(channelId)
  const view = d.kind === 'STICKINESS' ? 'STICKINESS' : 'RETENTION'
  return (
    <div className="space-y-3">
    <div className="flex flex-wrap items-end gap-3 text-sm">
      <Field label="View">
        <Toggle
          label="View"
          options={[
            { v: 'STICKINESS', l: 'Return rate' },
            { v: 'RETENTION', l: 'By cohort' },
          ]}
          value={view}
          onChange={(v) => onChange(switchRetentionView(d, v))}
        />
      </Field>
      {view === 'STICKINESS' ? (
        <>
          <Field label="Active means doing">
            <EventSelect label="Active means doing" value={d.event ?? ''} onChange={(event) => onChange({ ...d, event })} events={events} anyLabel="Any activity" />
          </Field>
          <Field label="Return window">
            <select
              aria-label="Return window"
              value={d.windowDays ?? ''}
              onChange={(e) => onChange({ ...d, windowDays: e.target.value ? Number(e.target.value) : undefined })}
              className="rounded border bg-background px-2 py-1.5 text-sm"
            >
              <option value="">The shop's default</option>
              {WINDOWS.map((w) => (
                <option key={w} value={w}>
                  {windowLabel(w)} ({w}d)
                </option>
              ))}
            </select>
          </Field>
        </>
      ) : (
        <>
          <Field label="Start event">
            <EventSelect label="Start event" value={d.startEvent ?? ''} onChange={(startEvent) => onChange({ ...d, startEvent })} events={events} />
          </Field>
          <Field label="Came back & did">
            <EventSelect label="Came back and did" value={d.returnEvent ?? ''} onChange={(returnEvent) => onChange({ ...d, returnEvent })} events={events} />
          </Field>
          <Field label="By">
            <Toggle
              label="By"
              options={[
                { v: 'day', l: 'Day' },
                { v: 'week', l: 'Week' },
              ]}
              value={d.unit ?? UNIT}
              onChange={(unit) => onChange({ ...d, unit, periods: periodsFor(unit) })}
            />
          </Field>
        </>
      )}
    </div>
    {view === 'STICKINESS' ? (
      <ConditionsEditor channelId={channelId} events={d.event ? [d.event] : []} label="Where" value={d.where} onChange={(where) => onChange({ ...d, where })} />
    ) : (
      <div className="grid gap-3 sm:grid-cols-2">
        <ConditionsEditor channelId={channelId} events={d.startEvent ? [d.startEvent] : []} label="Where the start event" value={d.startWhere} onChange={(startWhere) => onChange({ ...d, startWhere })} />
        <ConditionsEditor channelId={channelId} events={d.returnEvent ? [d.returnEvent] : []} label="Where the return event" value={d.returnWhere} onChange={(returnWhere) => onChange({ ...d, returnWhere })} />
      </div>
    )}
    </div>
  )
}

function RetentionResult({ channelId, d, sinceDays, compact }: ResultProps) {
  const unit = d.unit ?? UNIT
  const periods = d.periods ?? periodsFor(unit)
  const { data, loading, error } = useQuery(SynapseRetentionDocument, {
    variables: { channelId, startEvent: d.startEvent ?? '', returnEvent: d.returnEvent ?? '', sinceDays, periods, unit, startFilters: readyConditions(d.startWhere), returnFilters: readyConditions(d.returnWhere) },
    skip: !channelId || !d.startEvent || !d.returnEvent,
    fetchPolicy: 'cache-and-network',
  })
  const ret = data?.synapseRetention
  const unitLabel = unit === 'day' ? 'Day' : 'Wk'
  if (!d.startEvent || !d.returnEvent) return <Note>Pick a start and a return event.</Note>

  if (compact) {
    const buckets = (ret?.buckets ?? []).filter((b) => b.period > 0)
    return (
      <State loading={loading && !ret} error={error}>
        <div className="space-y-3">
          <Headline value={ret?.cohort ?? 0} label="people started" />
          <div className="grid grid-cols-4 gap-1 text-center text-xs sm:grid-cols-8">
            {buckets.map((b) => (
              <div key={b.period} className="rounded bg-muted px-1 py-1.5">
                <div className="text-muted-foreground">
                  {unit === 'week' ? 'W' : 'D'}
                  {b.period}
                </div>
                <div className="font-medium tabular-nums">{b.eligible > 0 ? `${pct(b.count, b.eligible)}%` : '-'}</div>
              </div>
            ))}
          </div>
        </div>
      </State>
    )
  }

  const p1 = pctOf(ret?.buckets[1])
  const lastMature = [...(ret?.buckets ?? [])].reverse().find((b) => b.eligible > 0)
  const lastPct = pctOf(lastMature)
  return (
    <State loading={loading && !ret} error={error}>
      {!ret || ret.cohort === 0 ? (
        <Note>No one did “{eventLabel(d.startEvent)}” in this period - pick another start event.</Note>
      ) : (
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <StatTile label="Cohort" value={formatNumber(ret.cohort)} sub={`did ${eventLabel(d.startEvent)}`} />
            <StatTile label={`${unitLabel} 1 retention`} value={p1 === null ? '-' : `${p1}%`} sub={`came back & did ${eventLabel(d.returnEvent)}`} />
            <StatTile label={lastMature ? `${unitLabel} ${lastMature.period} retention` : 'Latest'} value={lastPct === null ? '-' : `${lastPct}%`} sub="most recent mature period" />
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
                  <CohortRow key={c.startAt} label={fmtDay(c.startAt)} size={c.size} buckets={c.buckets} />
                ))}
              </tbody>
            </table>
          </div>
          <p className="text-xs text-muted-foreground">
            Blank cells are periods a cohort is too new to have reached, and each percentage counts only the members old enough
            for it. Cohorts of fewer than {SMALL_COHORT} people are faded: one person moves them a lot.
          </p>
        </div>
      )}
    </State>
  )
}

function CohortRow({ label, size, buckets, emphasize }: { label: string; size: number; buckets: Bucket[]; emphasize?: boolean }) {
  const small = !emphasize && size < SMALL_COHORT
  return (
    <tr className={cn('border-t', emphasize && 'bg-muted/40 font-medium', small && 'opacity-50')} title={small ? `Only ${size} people - too few to read much into` : undefined}>
      <td className={cn('sticky left-0 z-10 whitespace-nowrap px-3 py-1.5', emphasize ? 'bg-muted/40 font-medium' : 'bg-background')}>{label}</td>
      <td className="px-2 py-1.5 text-right tabular-nums text-muted-foreground">{formatNumber(size)}</td>
      {buckets.map((b) => {
        const p = pctOf(b)
        return (
          <td key={b.period} className="px-0.5 py-1">
            {p === null ? (
              <div className="mx-auto grid h-8 w-12 place-items-center rounded bg-muted/20 text-muted-foreground/30">·</div>
            ) : (
              <div className="relative mx-auto grid h-8 w-12 place-items-center overflow-hidden rounded" title={`${formatNumber(b.count)} of ${formatNumber(b.eligible)}`}>
                <div className="absolute inset-0 bg-series-1" style={{ opacity: Math.max(0.06, p / 100) }} aria-hidden />
                <span className={cn('relative text-[11px] tabular-nums', p >= 55 ? 'text-white' : 'text-foreground')}>{p}%</span>
              </div>
            )}
          </td>
        )
      })}
    </tr>
  )
}

function StickinessResult({ channelId, d, sinceDays, compact }: ResultProps) {
  const { data, loading, error } = useQuery(SynapseStickinessDocument, {
    variables: { channelId, event: d.event || null, sinceDays, windowDays: d.windowDays ?? null, filters: readyConditions(d.where) },
    skip: !channelId,
    fetchPolicy: 'cache-and-network',
  })
  const s = data?.synapseStickiness
  const window = s?.windowDays ?? d.windowDays ?? 30
  const returnPct = Math.round(s?.returnRate ?? 0)

  if (compact) {
    return (
      <State loading={loading && !s} error={error}>
        <div className="space-y-3">
          <Headline value={`${returnPct}%`} label={`came back within ${window} days`} />
          <div className="flex gap-4 text-xs text-muted-foreground">
            <span>
              Weekly active <span className="font-medium tabular-nums text-foreground">{formatNumber(s?.wau ?? 0)}</span>
            </span>
            <span>
              Monthly active <span className="font-medium tabular-nums text-foreground">{formatNumber(s?.mau ?? 0)}</span>
            </span>
          </div>
          <Bars values={(s?.days ?? []).map((p) => p.count)} />
        </div>
      </State>
    )
  }

  const returning = s?.returning ?? 0
  const activeNow = s?.activeNow ?? 0
  const retShare = pct(returning, activeNow)
  const max = Math.max(1, ...(s?.days ?? []).map((x) => x.count))
  return (
    <State loading={loading && !s} error={error}>
      {!s || s.priorActive === 0 ? (
        <Note>No activity in the previous {window}-day window to measure returns against yet - widen the window or wait for more history.</Note>
      ) : (
        <div className="space-y-4">
          <div className="grid gap-4 lg:grid-cols-[260px_1fr]">
            <div className="rounded-lg border p-4">
              <div className="text-5xl font-semibold leading-none tabular-nums">{returnPct}%</div>
              <div className="mt-2 text-sm font-medium">came back</div>
              <div className="mt-1 text-xs text-muted-foreground">
                {formatNumber(returning)} of {formatNumber(s.priorActive)} people active in the prior {window} days returned in the last {window}.
              </div>
            </div>
            <div className="rounded-lg border p-4">
              <div className="mb-2 flex items-baseline justify-between">
                <span className="text-sm font-medium">Active in the last {window} days</span>
                <span className="text-lg font-semibold tabular-nums">{formatNumber(activeNow)}</span>
              </div>
              <div className="flex h-2.5 overflow-hidden rounded-full bg-muted" role="img" aria-label={`${retShare}% returning, ${100 - retShare}% new or reactivated`}>
                <span className="bg-series-1" style={{ width: `${retShare}%` }} />
                <span className="bg-series-2" style={{ width: `${100 - retShare}%` }} />
              </div>
              <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
                <div className="flex items-center gap-2">
                  <span className="size-2.5 rounded-full bg-series-1" />
                  <span className="text-muted-foreground">Returning</span>
                  <span className="ml-auto font-medium tabular-nums">{formatNumber(returning)}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="size-2.5 rounded-full bg-series-2" />
                  <span className="text-muted-foreground">New or reactivated</span>
                  <span className="ml-auto font-medium tabular-nums">{formatNumber(Math.max(0, activeNow - returning))}</span>
                </div>
              </div>
              <p className="mt-3 border-t pt-2 text-xs text-muted-foreground">
                For reference: {formatNumber(s.wau)} active in the last 7 days · {formatNumber(s.mau)} in the last 30.
              </p>
            </div>
          </div>
          {s.days.length > 0 && (
            <div className="rounded border p-3">
              <p className="mb-2 text-xs text-muted-foreground">Active people per day</p>
              <div className="flex h-36 items-end gap-0.5">
                {s.days.map((x) => (
                  <div key={x.day} title={`${fmtDay(x.day)}: ${formatNumber(x.count)}`} className="flex-1 rounded-t bg-series-1/70 hover:bg-series-1" style={{ height: `${Math.max(2, Math.round((x.count / max) * 100))}%` }} />
                ))}
              </div>
              <div className="mt-1.5 flex justify-between text-[11px] text-muted-foreground">
                <span>{fmtDay(s.days[0].day)}</span>
                <span>{fmtDay(s.days[s.days.length - 1].day)}</span>
              </div>
            </div>
          )}
        </div>
      )}
    </State>
  )
}

export const retention: Kind = {
  page: '/synapse/retention',
  label: 'Retention by cohort',
  menu: { label: 'Retention', hint: 'Whether people come back' },
  describe: (d) =>
    `${d.startEvent}${d.startWhere?.length ? ` where ${describeConditions(d.startWhere)}` : ''} then ${d.returnEvent}${d.returnWhere?.length ? ` where ${describeConditions(d.returnWhere)}` : ''}, by ${d.unit ?? UNIT}`,
  title: (d) => `${eventLabel(d.startEvent ?? '')} retention`,
  start: { kind: 'RETENTION', startEvent: 'order_placed', returnEvent: 'order_placed', unit: UNIT, periods: periodsFor(UNIT) },
  Controls: RetentionControls,
  Result: RetentionResult,
}

export const stickiness: Kind = {
  page: '/synapse/stickiness',
  label: 'Return rate',
  builder: 'RETENTION',
  describe: (d) => `Returning on ${d.event || 'any activity'}${d.where?.length ? ` where ${describeConditions(d.where)}` : ''}`,
  title: (d) => (d.event ? `Returning on ${eventLabel(d.event)}` : 'Returning shoppers'),
  start: { kind: 'STICKINESS' },
  Controls: RetentionControls,
  Result: StickinessResult,
}
