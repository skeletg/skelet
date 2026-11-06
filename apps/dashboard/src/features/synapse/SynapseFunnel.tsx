import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ChevronDownIcon, ChevronUpIcon, FilterIcon, XIcon } from 'lucide-react'
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
import { SynapseFunnelDocument, SynapseOverviewDocument } from './Synapse.gql'
import { formatNumber } from '@/lib/format'
import { AddToBoard } from '@/features/boards/AddToBoard'

// A step's own filters travel with it when it moves or goes
type StepFilter = { key: string; value: string }
type Step = { event: string; filters: StepFilter[] }

const DEFAULT_STEPS: Step[] = ['page_view', 'view_item', 'cart_add', 'begin_checkout', 'order_placed'].map((event) => ({ event, filters: [] }))
const pct = (n: number, d: number) => (d > 0 ? Math.round((n / d) * 100) : 0)

const WINDOWS: { label: string; hours: number | null }[] = [
  { label: 'Any time in the period', hours: null },
  { label: 'Within 1 hour', hours: 1 },
  { label: 'Within 1 day', hours: 24 },
  { label: 'Within 7 days', hours: 24 * 7 },
  { label: 'Within 30 days', hours: 24 * 30 },
]

export function SynapseFunnel() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [stepList, setSteps] = useState<Step[]>(DEFAULT_STEPS)
  const [windowHours, setWindowHours] = useState<number | null>(null)
  const [breakdownDraft, setBreakdownDraft] = useState('')
  const [breakdown, setBreakdown] = useState('')
  const [filtering, setFiltering] = useState<number | null>(null) // the step whose filter form is open
  const steps = stepList.map((s) => s.event)
  const filters = stepList.flatMap((s, step) => s.filters.map((f) => ({ step, ...f })))

  // Available event names to build steps from = whatever this channel has recorded
  const { data: ov, error: ovError } = useQuery(SynapseOverviewDocument, {
    variables: { channelId, sinceDays: 30 },
    skip: !isChannel,
    fetchPolicy: 'cache-and-network',
  })
  const available = (ov?.synapseOverview.topEvents ?? []).map((e) => e.name)

  const { data, loading, error: funnelError } = useQuery(SynapseFunnelDocument, {
    variables: { channelId, steps, sinceDays: 30, windowHours, filters, breakdown: breakdown || null },
    skip: !isChannel || steps.length < 2,
    fetchPolicy: 'cache-and-network',
  })
  const error = ovError ?? funnelError
  const result = data?.synapseFunnel.steps ?? []
  const split = data?.synapseFunnel.breakdown ?? []
  const top = result[0]?.count ?? 0

  const add = (e: string) => e && setSteps((s) => [...s, { event: e, filters: [] }])
  const addFilter = (i: number, f: StepFilter) =>
    setSteps((s) => s.map((st, k) => (k === i ? { ...st, filters: [...st.filters, f] } : st)))
  const removeFilter = (i: number, j: number) =>
    setSteps((s) => s.map((st, k) => (k === i ? { ...st, filters: st.filters.filter((_, n) => n !== j) } : st)))
  const remove = (i: number) => {
    setFiltering(null)
    setSteps((s) => s.filter((_, k) => k !== i))
  }
  const move = (i: number, d: -1 | 1) => {
    setFiltering(null)
    setSteps((s) => {
      const j = i + d
      if (j < 0 || j >= s.length) return s
      const next = [...s]
      ;[next[i], next[j]] = [next[j], next[i]]
      return next
    })
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Funnel"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        actions={isChannel ? <AddToBoard channelId={channelId} title={steps.map(eventLabel).join(' → ')} definition={{ kind: 'FUNNEL', steps, filters, windowHours: windowHours ?? undefined, breakdown }} /> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Ordered conversion - how many people complete each step in sequence, within the conversion window. People
            are resolved across their anonymous + signed-in activity. Last 30 days.
          </span>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load funnel</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Funnels are per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection title="Steps" description="Add events in order; each step must happen after the one before it.">
              <div className="space-y-2">
                {stepList.map((st, i) => (
                  <div key={`${st.event}-${i}`} className="rounded border px-3 py-2 text-sm">
                    <div className="flex items-center gap-2">
                      <span className="w-5 shrink-0 text-center text-xs font-semibold text-muted-foreground">{i + 1}</span>
                      <span className="flex-1 font-medium">{eventLabel(st.event)}</span>
                      <span className="text-xs text-muted-foreground">{st.event}</span>
                      <button type="button" aria-label={`Filter step ${i + 1}`} onClick={() => setFiltering(filtering === i ? null : i)} className="rounded p-1 text-muted-foreground hover:bg-muted">
                        <FilterIcon className="size-4" />
                      </button>
                      <button type="button" aria-label={`Move step ${i + 1} up`} onClick={() => move(i, -1)} disabled={i === 0} className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30">
                        <ChevronUpIcon className="size-4" />
                      </button>
                      <button type="button" aria-label={`Move step ${i + 1} down`} onClick={() => move(i, 1)} disabled={i === stepList.length - 1} className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30">
                        <ChevronDownIcon className="size-4" />
                      </button>
                      <button type="button" aria-label={`Remove step ${i + 1}`} onClick={() => remove(i)} className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground">
                        <XIcon className="size-4" />
                      </button>
                    </div>
                    {st.filters.length > 0 && (
                      <div className="mt-1.5 flex flex-wrap gap-1.5 pl-7">
                        {st.filters.map((f, j) => (
                          <span key={`${f.key}-${j}`} className="inline-flex items-center gap-1 rounded bg-muted px-2 py-0.5 text-xs">
                            <span className="font-mono">{f.key}</span> = <span className="font-mono">{f.value}</span>
                            <button type="button" aria-label={`Remove filter ${f.key} from step ${i + 1}`} onClick={() => removeFilter(i, j)} className="text-muted-foreground hover:text-foreground">
                              <XIcon className="size-3" />
                            </button>
                          </span>
                        ))}
                      </div>
                    )}
                    {filtering === i && (
                      <StepFilterForm
                        step={i + 1}
                        onAdd={(f) => {
                          addFilter(i, f)
                          setFiltering(null)
                        }}
                      />
                    )}
                  </div>
                ))}
                <div className="flex items-center gap-2 pt-1">
                  <select
                    value=""
                    onChange={(e) => {
                      add(e.target.value)
                      e.currentTarget.value = ''
                    }}
                    className="rounded border bg-background px-2 py-1.5 text-sm"
                  >
                    <option value="" disabled>
                      + Add step…
                    </option>
                    {available.map((e) => (
                      <option key={e} value={e}>
                        {eventLabel(e)}
                      </option>
                    ))}
                  </select>
                  {steps.length < 2 && <span className="text-xs text-muted-foreground">Add at least 2 steps.</span>}
                </div>
                <div className="flex flex-wrap items-center gap-4 border-t pt-3">
                  <label className="flex items-center gap-2 text-sm">
                    <span className="text-muted-foreground">Conversion window</span>
                    <select
                      value={windowHours ?? ''}
                      onChange={(e) => setWindowHours(e.target.value ? Number(e.target.value) : null)}
                      className="rounded border bg-background px-2 py-1.5 text-sm"
                    >
                      {WINDOWS.map((w) => (
                        <option key={w.label} value={w.hours ?? ''}>
                          {w.label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <form
                    className="flex items-center gap-2 text-sm"
                    onSubmit={(e) => {
                      e.preventDefault()
                      setBreakdown(breakdownDraft.trim())
                    }}
                  >
                    <label className="flex items-center gap-2">
                      <span className="text-muted-foreground">Break down by</span>
                      <input
                        value={breakdownDraft}
                        onChange={(e) => setBreakdownDraft(e.target.value)}
                        onBlur={() => setBreakdown(breakdownDraft.trim())}
                        placeholder="a property of step 1, e.g. device"
                        className="w-64 rounded border bg-background px-2 py-1.5 text-sm"
                      />
                    </label>
                  </form>
                </div>
              </div>
            </DetailPageSection>

            <DetailPageSection title="Conversion">
              {steps.length < 2 ? (
                <p className="py-6 text-sm text-muted-foreground">Add at least two steps to see the funnel.</p>
              ) : loading && result.length === 0 ? (
                <p className="py-6 text-sm text-muted-foreground">Computing…</p>
              ) : (
                <div className="space-y-2">
                  {result.map((s, i) => {
                    const overall = pct(s.count, top)
                    const prev = i > 0 ? result[i - 1].count : s.count
                    const fromPrev = pct(s.count, prev)
                    return (
                      <div key={`${s.eventName}-${i}`} className="rounded border p-3">
                        <div className="mb-1.5 flex items-baseline justify-between gap-3 text-sm">
                          <span className="font-medium">
                            <span className="mr-2 text-xs text-muted-foreground">{i + 1}</span>
                            {eventLabel(s.eventName)}
                          </span>
                          <span className="tabular-nums">
                            {formatNumber(s.count)}
                            <span className="ml-2 text-xs text-muted-foreground">{overall}%</span>
                          </span>
                        </div>
                        <div className="h-2 overflow-hidden rounded-full bg-muted">
                          <div className="h-full rounded-full bg-primary" style={{ width: `${overall}%` }} />
                        </div>
                        {i > 0 && (
                          <div className={cn('mt-1 text-xs', fromPrev < 50 ? 'text-warning' : 'text-muted-foreground')}>
                            {fromPrev}% from previous step · {100 - fromPrev}% drop-off
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              )}
            </DetailPageSection>

            {breakdown && steps.length >= 2 && (
              <DetailPageSection title={`By ${breakdown}`} description="People by the property's value on their first step, the ten largest groups.">
                {split.length === 0 ? (
                  <p className="py-6 text-sm text-muted-foreground">No first-step events carry {breakdown}.</p>
                ) : (
                  <div className="overflow-x-auto rounded border">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b text-left text-muted-foreground">
                          <th className="px-3 py-2 font-medium">{breakdown}</th>
                          {steps.map((s, i) => (
                            <th key={`${s}-${i}`} className="px-3 py-2 text-right font-medium">
                              {i + 1}. {eventLabel(s)}
                            </th>
                          ))}
                          <th className="px-3 py-2 text-right font-medium">Converted</th>
                        </tr>
                      </thead>
                      <tbody>
                        {split.map((b) => (
                          <tr key={b.value} className="border-b last:border-0">
                            <td className="px-3 py-2 font-mono text-xs">{b.value}</td>
                            {b.counts.map((n, i) => (
                              <td key={i} className="px-3 py-2 text-right tabular-nums">
                                {formatNumber(n)}
                              </td>
                            ))}
                            <td className="px-3 py-2 text-right tabular-nums">{pct(b.counts[b.counts.length - 1] ?? 0, b.counts[0] ?? 0)}%</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </DetailPageSection>
            )}
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// StepFilterForm holds one step to events carrying a property with a value
function StepFilterForm({ step, onAdd }: { step: number; onAdd: (f: StepFilter) => void }) {
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  return (
    <form
      className="mt-2 flex flex-wrap items-center gap-2 pl-7"
      onSubmit={(e) => {
        e.preventDefault()
        if (key.trim()) onAdd({ key: key.trim(), value })
      }}
    >
      <input aria-label={`Property for step ${step}`} value={key} onChange={(e) => setKey(e.target.value)} placeholder="property, e.g. device" className="w-48 rounded border bg-background px-2 py-1 text-xs" />
      <span className="text-xs text-muted-foreground">=</span>
      <input aria-label={`Value for step ${step}`} value={value} onChange={(e) => setValue(e.target.value)} placeholder="value" className="w-40 rounded border bg-background px-2 py-1 text-xs" />
      <button type="submit" disabled={!key.trim()} className="rounded border px-2 py-1 text-xs hover:bg-muted disabled:opacity-40">
        Add filter
      </button>
    </form>
  )
}
