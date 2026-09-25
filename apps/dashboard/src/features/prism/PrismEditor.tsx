import { useState, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import { CheckIcon, ChevronsUpDownIcon, PlusIcon } from 'lucide-react'
import { Bar, BarChart, CartesianGrid, Cell, ErrorBar, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { LoadingState } from '@/components/ui/loading-state'
import { CHART_ACCENT, CHART_SUCCESS, SERIES_OTHER } from '@/lib/chart-colors'
import { cn } from '@/lib/utils'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { PrismSegmentsDocument } from './Prism.gql'
import { CreateExperimentDocument, DeleteExperimentDocument, ExperimentDocument, ExperimentGoalEventsDocument, ExperimentResultsDocument, UpdateExperimentDocument } from './Prism.gql'
import { PluginSections } from '@/plugins/detail-sections'

type Variation = { key: string; name: string; weight: number; value: string }
type Draft = {
  id: string | null
  channelId: string
  key: string
  name: string
  status: string
  coverage: number
  goalEvent: string
  segmentSlug: string
  variations: Variation[]
}

export function PrismEditor() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { effectiveChannelId, isChannel } = useConfigScope()
  const isNew = !id || id === 'new'
  const { data, loading } = useQuery(ExperimentDocument, { variables: { id: id ?? '' }, skip: isNew, fetchPolicy: 'cache-and-network' })

  if (isNew && !isChannel) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="New experiment" backTo="/prism/experiments" />
        <DetailPageBody>
          <DetailPageSection>
            <Alert>
              <AlertTitle>Pick a channel</AlertTitle>
              <AlertDescription>Select a channel in the header before creating an experiment.</AlertDescription>
            </Alert>
          </DetailPageSection>
        </DetailPageBody>
      </DetailPageLayout>
    )
  }
  if (!isNew && !data?.experiment) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="Experiment" backTo="/prism/experiments" />
        <DetailPageBody>{loading ? <LoadingState /> : null}</DetailPageBody>
      </DetailPageLayout>
    )
  }

  const e = data?.experiment
  const initial: Draft = isNew
    ? {
        id: null, channelId: effectiveChannelId ?? '', key: '', name: '', status: 'draft', coverage: 100, goalEvent: '',
        segmentSlug: '',
        variations: [
          { key: 'control', name: 'Control', weight: 1, value: '' },
          { key: 'variant', name: 'Variant', weight: 1, value: '' },
        ],
      }
    : {
        id: e!.id, channelId: e!.channelId, key: e!.key, name: e!.name, status: e!.status, coverage: e!.coverage,
        goalEvent: e!.goalEvent, segmentSlug: e!.segmentSlug, variations: e!.variations.map((v) => ({ key: v.key, name: v.name, weight: v.weight, value: v.value })),
      }

  return <EditorInner key={initial.id ?? 'new'} initial={initial} onSaved={(nid) => void duringCommit(() => navigate(nid ? `/prism/${nid}` : '/prism/experiments'))} />
}

function EditorInner({ initial, onSaved }: { initial: Draft; onSaved: (id: string) => void }) {
  const draft = useDraft({
    key: initial.key,
    name: initial.name,
    status: initial.status,
    coverage: initial.coverage,
    goalEvent: initial.goalEvent,
    segmentSlug: initial.segmentSlug,
    variations: initial.variations as Variation[],
  })
  const { key, name, status, coverage, goalEvent, segmentSlug, variations } = draft.values
  const { setKey, setName, setStatus, setCoverage, setGoalEvent, setSegmentSlug, setVariations } = draft.setters
  const [expId, setExpId] = useState<string | null>(initial.id)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [createExp] = useMutation(CreateExperimentDocument)
  const [updateExp] = useMutation(UpdateExperimentDocument)
  const [deleteExp] = useMutation(DeleteExperimentDocument)

  // Goal-event suggestions: the custom events the storefront has actually fired
  // for this channel - The field stays free-text (a goal may not have fired yet)
  const { data: goalData } = useQuery(ExperimentGoalEventsDocument, {
    variables: { channelId: initial.channelId },
    skip: !initial.channelId,
    fetchPolicy: 'cache-and-network',
  })
  const goalEvents = goalData?.experimentGoalEvents ?? []

  // Active customer segments for optional targeting (decoupled query gated on
  // experiments.view - doesn't need promotions.view)
  const { data: segData } = useQuery(PrismSegmentsDocument, { fetchPolicy: 'cache-and-network' })
  const segments = segData?.experimentSegments ?? []

  const setVar = (i: number, patch: Partial<Variation>) =>
    setVariations((vs) => vs.map((v, j) => (j === i ? { ...v, ...patch } : v)))
  const addVar = () => setVariations((vs) => [...vs, { key: `v${vs.length}`, name: `Variation ${vs.length}`, weight: 1, value: '' }])
  const removeVar = (i: number) => setVariations((vs) => (vs.length > 2 ? vs.filter((_, j) => j !== i) : vs))

  function input() {
    return {
      channelId: initial.channelId,
      key: key.trim(),
      name: name.trim(),
      status,
      coverage,
      goalEvent: goalEvent.trim(),
      segmentSlug,
      variations: variations.map((v) => ({ key: v.key.trim(), name: v.name.trim(), weight: v.weight, value: v.value })),
    }
  }

  async function save() {
    setError(null)
    setSaving(true)
    try {
      if (expId) {
        await updateExp({ variables: { id: expId, input: input() } })
        draft.saved() // stays on the page, so the baseline has to move with it
      } else {
        const res = await createExp({ variables: { input: input() } })
        const nid = res.data?.createExperiment?.id
        if (nid) {
          setExpId(nid)
          onSaved(nid)
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  async function remove() {
    if (!expId) return
    await deleteExp({ variables: { id: expId } })
    onSaved('')
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title={initial.id ? initial.name || 'Experiment' : 'New experiment'}
        backTo="/prism/experiments"
        actions={
          <div className="flex gap-2">
            {expId && (
              <Button variant="outline" size="sm" onClick={remove}>
                Delete
              </Button>
            )}
            <Button size="sm" onClick={save} disabled={saving}>
              {saving ? 'Saving…' : 'Save'}
            </Button>
          </div>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive" className="mx-6 mt-4">
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {/* Controls (left) and results (right) side by side - neither needs the
            full width - Collapses to a single stacked column below lg */}
        <div className="grid items-start lg:grid-cols-2">
          <div className="divide-y">
            <DetailPageSection title="Setup">
          <div className="grid max-w-md gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="ename">Name</Label>
              <Input id="ename" value={name} onChange={(ev) => setName(ev.target.value)} placeholder="Hero CTA test" />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="ekey">Key</Label>
              <Input id="ekey" value={key} onChange={(ev) => setKey(ev.target.value)} placeholder="hero-cta" disabled={!!expId} />
              <p className="text-xs text-muted-foreground">Stable id used in the storefront (<code className="text-[11px]">useExperiment(&apos;{key || 'hero-cta'}&apos;)</code>) and in hashing - can&apos;t change after creation.</p>
            </div>
            <div className="flex flex-wrap gap-3">
              <div className="grid gap-1.5">
                <Label>Status</Label>
                <Select value={status} onValueChange={setStatus}>
                  <SelectTrigger className="w-36"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="draft">Draft</SelectItem>
                    <SelectItem value="running">Running</SelectItem>
                    <SelectItem value="stopped">Stopped</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="ecov">Coverage %</Label>
                <Input id="ecov" type="number" min={0} max={100} value={coverage} onChange={(ev) => setCoverage(Number(ev.target.value))} className="w-28" />
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label>Goal event</Label>
              <GoalEventPicker value={goalEvent} onChange={setGoalEvent} events={goalEvents} />
              <p className="text-xs text-muted-foreground">
                Search a tracked event or type a new name. The conversion is counted when the storefront calls{' '}
                <code className="text-[11px]">window.lens.track(&apos;{goalEvent || 'purchase'}&apos;)</code> after
                exposure.
              </p>
            </div>
            <div className="grid gap-1.5">
              <Label>Target audience</Label>
              <Select value={segmentSlug || '__all__'} onValueChange={(v) => setSegmentSlug(v === '__all__' ? '' : v)}>
                <SelectTrigger className="w-72"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all__">All visitors</SelectItem>
                  {segmentSlug && !segments.some((s) => s.slug === segmentSlug) && (
                    <SelectItem value={segmentSlug}>{segmentSlug}</SelectItem>
                  )}
                  {segments.map((s) => (
                    <SelectItem key={s.slug} value={s.slug}>
                      {s.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                Limit the experiment to a customer segment - resolved server-side, so only logged-in customers in the
                segment are entered (anonymous visitors aren&apos;t). Default: everyone.
              </p>
            </div>
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Variations"
          description="The first is the control. Weights set the split. value is the payload the storefront renders (useExperiment returns it)."
          trailing={<Button variant="outline" size="sm" onClick={addVar}>Add variation</Button>}
        >
          <div className="grid gap-2">
            {variations.map((v, i) => (
              <div key={i} className="flex flex-wrap items-end gap-2 rounded border p-3">
                <div className="grid gap-1">
                  <Label className="text-xs">Key</Label>
                  <Input value={v.key} onChange={(ev) => setVar(i, { key: ev.target.value })} className="w-28" />
                </div>
                <div className="grid gap-1">
                  <Label className="text-xs">Name</Label>
                  <Input value={v.name} onChange={(ev) => setVar(i, { name: ev.target.value })} className="w-32" />
                </div>
                <div className="grid gap-1">
                  <Label className="text-xs">Weight</Label>
                  <Input type="number" min={0} value={v.weight} onChange={(ev) => setVar(i, { weight: Number(ev.target.value) })} className="w-20" />
                </div>
                <div className="grid flex-1 gap-1">
                  <Label className="text-xs">Value{i === 0 ? ' (control)' : ''}</Label>
                  <Input value={v.value} onChange={(ev) => setVar(i, { value: ev.target.value })} placeholder="Buy now" />
                </div>
                {variations.length > 2 && (
                  <Button variant="ghost" size="sm" onClick={() => removeVar(i)}>✕</Button>
                )}
              </div>
            ))}
          </div>
          <p className="mt-3 text-xs text-muted-foreground">
            Preview an arm without affecting results: append{' '}
            <code className="text-[11px]">?prism={key || 'hero-cta'}:{variations[1]?.key || 'variant'}</code> to any
            storefront URL (sticks for the session; <code className="text-[11px]">?prism=off</code> clears it). Forced
            views don&apos;t fire exposures.
          </p>
            </DetailPageSection>
          </div>

          <div className="divide-y border-t lg:border-l lg:border-t-0">
            {expId && initial.status !== 'draft' ? (
              <Results id={expId} goalEvent={initial.goalEvent} />
            ) : (
              <DetailPageSection title="Results">
                <p className="py-4 text-sm text-muted-foreground">
                  {expId
                    ? 'This experiment is a draft. Set status to “Running” and save to start collecting exposures and conversions.'
                    : 'Save the experiment to start collecting results.'}
                </p>
              </DetailPageSection>
            )}
          </div>
        </div>
        <PluginSections target="experiment" entity={initial} mode={initial.id ? 'edit' : 'create'} />
      </DetailPageBody>
    </DetailPageLayout>
  )
}

type ChartRow = {
  name: string
  ratePct: number
  ci: [number, number]
  isControl: boolean
  isWinner: boolean
  exposed: number
  converted: number
}

function Results({ id, goalEvent }: { id: string; goalEvent: string }) {
  const { data } = useQuery(ExperimentResultsDocument, { variables: { id, sinceDays: 30 }, fetchPolicy: 'cache-and-network' })
  const res = data?.experimentResults
  const rows = res?.variations ?? []
  const pct = (x: number) => `${(x * 100).toFixed(1)}%`
  const money = (cents: number) => (cents / 100).toLocaleString(undefined, { style: 'currency', currency: 'USD' })
  const hasRevenue = (res?.totalRevenue ?? 0) > 0
  const winnerName = res?.winner ? rows.find((r) => r.variation === res.winner)?.name || res.winner : ''

  const chartData: ChartRow[] = rows.map((r) => ({
    name: r.name || r.variation,
    ratePct: +(r.rate * 100).toFixed(2),
    ci: [+((r.rate - r.ciLow) * 100).toFixed(2), +((r.ciHigh - r.rate) * 100).toFixed(2)],
    isControl: r.isControl,
    isWinner: res?.winner === r.variation,
    exposed: r.exposed,
    converted: r.converted,
  }))
  const maxY = Math.min(100, Math.max(1, Math.ceil(Math.max(0, ...rows.map((r) => r.ciHigh)) * 100 * 1.15)))
  const challengers = rows.filter((r) => !r.isControl)

  return (
    <DetailPageSection
      title="Results"
      description={`Conversion by variation (goal: ${goalEvent || '-'}), last 30 days · ${res?.totalExposed ?? 0} exposed${hasRevenue ? ` · ${money(res?.totalRevenue ?? 0)} revenue` : ''}.`}
    >
      {res?.sampleRatioMismatch && (
        <Alert variant="destructive" className="mb-3">
          <AlertTitle>Sample ratio mismatch</AlertTitle>
          <AlertDescription>
            The observed traffic split doesn&apos;t match the configured weights - randomization or tracking is likely
            broken. Treat these numbers with caution until it&apos;s resolved.
          </AlertDescription>
        </Alert>
      )}
      {rows.length === 0 ? (
        <p className="py-4 text-sm text-muted-foreground">No exposures yet.</p>
      ) : (
        <>
          <div
            className={cn(
              'mb-4 rounded border px-3 py-2 text-sm',
              winnerName ? 'border-success/40 bg-success-soft text-success' : 'text-muted-foreground',
            )}
          >
            {winnerName ? (
              <>
                🏆 <span className="font-medium">{winnerName}</span> is winning - ≥97.5% chance to beat the control.
              </>
            ) : (
              'No statistically significant winner yet - keep running until a variation clears 97.5% chance to win.'
            )}
          </div>

          <div className="mb-4 grid gap-4">
            <Panel title="Conversion rate (95% CI)">
              <div className="h-56">
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart data={chartData} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
                    <CartesianGrid vertical={false} strokeDasharray="3 3" className="stroke-muted" />
                    <XAxis dataKey="name" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
                    <YAxis domain={[0, maxY]} tickFormatter={(v) => `${v}%`} tick={{ fontSize: 11 }} tickLine={false} axisLine={false} width={36} />
                    <Tooltip content={<RateTooltip />} cursor={{ fill: 'transparent' }} />
                    <Bar dataKey="ratePct" radius={[4, 4, 0, 0]} maxBarSize={72}>
                      {chartData.map((d, i) => (
                        <Cell key={i} fill={d.isWinner ? CHART_SUCCESS : d.isControl ? SERIES_OTHER : CHART_ACCENT} />
                      ))}
                      <ErrorBar dataKey="ci" width={4} strokeWidth={1.5} stroke="var(--color-muted-foreground)" direction="y" />
                    </Bar>
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </Panel>
            <Panel title="Chance to beat control">
              {challengers.length === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">Add a variation to compare against the control.</p>
              ) : (
                <div className="grid gap-3 py-1">
                  {challengers.map((r) => (
                    <ChanceBar key={r.variation} name={r.name || r.variation} chance={r.chanceToWin} exposed={r.exposed} />
                  ))}
                  <p className="text-[11px] text-muted-foreground">The marker is the 97.5% bar a challenger must clear to be called a winner.</p>
                </div>
              )}
            </Panel>
          </div>

          <div className="overflow-x-auto rounded border">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
                  <th className="px-3 py-2">Variation</th>
                  <th className="px-3 py-2 text-right">Exposed</th>
                  <th className="px-3 py-2 text-right">Converted</th>
                  {hasRevenue && <th className="px-3 py-2 text-right">Revenue</th>}
                  <th className="px-3 py-2 text-right">Conversion rate</th>
                  <th className="px-3 py-2 text-right">Uplift</th>
                  <th className="px-3 py-2 text-right">Chance to win</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => {
                  const strong = !r.isControl && r.exposed > 0 && r.chanceToWin >= 0.975
                  const weak = !r.isControl && r.exposed > 0 && r.chanceToWin <= 0.025
                  return (
                    <tr key={r.variation} className="border-b last:border-0">
                      <td className="px-3 py-2 font-medium">
                        {r.name || r.variation || '(unassigned)'}
                        {r.isControl && <span className="ml-1.5 text-xs font-normal text-muted-foreground">control</span>}
                        {res?.winner === r.variation && ' 🏆'}
                      </td>
                      <td className="px-3 py-2 text-right tabular-nums">{r.exposed}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{r.converted}</td>
                      {hasRevenue && (
                        <td className="px-3 py-2 text-right tabular-nums">{r.revenueCents > 0 ? money(r.revenueCents) : '-'}</td>
                      )}
                      <td className="px-3 py-2 text-right tabular-nums">
                        {r.exposed > 0 ? (
                          <>
                            <div>{pct(r.rate)}</div>
                            <div className="text-[11px] text-muted-foreground">
                              95% CI {pct(r.ciLow)}–{pct(r.ciHigh)}
                            </div>
                          </>
                        ) : (
                          '-'
                        )}
                      </td>
                      <td className="px-3 py-2 text-right tabular-nums">
                        {r.isControl || r.exposed === 0 ? (
                          '-'
                        ) : (
                          <span className={r.uplift > 0 ? 'text-success' : r.uplift < 0 ? 'text-destructive' : ''}>
                            {r.uplift > 0 ? '+' : ''}
                            {pct(r.uplift)}
                          </span>
                        )}
                      </td>
                      <td className="px-3 py-2 text-right tabular-nums">
                        {r.isControl || r.exposed === 0 ? (
                          '-'
                        ) : (
                          <span className={strong ? 'font-medium text-success' : weak ? 'text-destructive' : 'text-muted-foreground'}>
                            {pct(r.chanceToWin)}
                          </span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </>
      )}
    </DetailPageSection>
  )
}

// A bordered panel with a header strip - matches the flat skelet style used by
// the Lens / Prism hubs
function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded border">
      <div className="border-b px-3 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">{title}</div>
      <div className="p-3">{children}</div>
    </div>
  )
}

// Custom recharts tooltip so the bar shows rate + raw counts (not the raw CI
// array the default tooltip would print)
function RateTooltip({ active, payload }: { active?: boolean; payload?: Array<{ payload: ChartRow }> }) {
  if (!active || !payload?.length) return null
  const d = payload[0].payload
  return (
    <div className="rounded border bg-background px-2.5 py-1.5 text-xs shadow-sm">
      <div className="font-medium">{d.name}</div>
      <div className="tabular-nums">{d.ratePct.toFixed(1)}% conversion</div>
      <div className="tabular-nums text-muted-foreground">
        {d.converted}/{d.exposed} converted
      </div>
    </div>
  )
}

// A horizontal chance-to-win bar with the 97.5% significance threshold marked
function ChanceBar({ name, chance, exposed }: { name: string; chance: number; exposed: number }) {
  const win = chance >= 0.975
  const lose = chance <= 0.025
  const w = Math.max(1.5, Math.min(100, chance * 100))
  return (
    <div className="grid gap-1">
      <div className="flex items-center justify-between text-xs">
        <span className="truncate">{name}</span>
        <span className={cn('tabular-nums', win ? 'font-medium text-success' : lose ? 'text-destructive' : 'text-muted-foreground')}>
          {exposed === 0 ? '-' : `${(chance * 100).toFixed(1)}%`}
        </span>
      </div>
      <div className="relative h-2 overflow-hidden rounded-full bg-muted">
        {exposed > 0 && (
          <span
            className={cn('block h-full rounded-full', win ? 'bg-success' : lose ? 'bg-destructive' : 'bg-primary')}
            style={{ width: `${w}%` }}
          />
        )}
        <span className="absolute top-0 h-full w-px bg-foreground/40" style={{ left: '97.5%' }} title="97.5% significance" />
      </div>
    </div>
  )
}

// GoalEventPicker is a searchable combobox over the channel's tracked events
// that also lets you commit a brand-new name (a goal that hasn't fired yet)
function GoalEventPicker({ value, onChange, events }: { value: string; onChange: (v: string) => void; events: string[] }) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const q = search.trim()
  const showCustom = q.length > 0 && !events.some((e) => e.toLowerCase() === q.toLowerCase())
  const commit = (v: string) => {
    onChange(v)
    setOpen(false)
    setSearch('')
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" role="combobox" aria-expanded={open} className="w-full justify-between font-normal">
          <span className={cn('min-w-0 truncate', !value && 'text-muted-foreground')}>
            {value || 'Select or type a goal event…'}
          </span>
          <ChevronsUpDownIcon className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start">
        <Command shouldFilter>
          <CommandInput placeholder="Search or type a new event…" value={search} onValueChange={setSearch} />
          <CommandList>
            <CommandEmpty>Type a name to create a new goal event.</CommandEmpty>
            {showCustom && (
              <CommandGroup heading="New">
                <CommandItem value={q} onSelect={() => commit(q)}>
                  <PlusIcon className="mr-2 size-4" />
                  Use “{q}”
                </CommandItem>
              </CommandGroup>
            )}
            {events.length > 0 && (
              <CommandGroup heading="Tracked events">
                {events.map((e) => (
                  <CommandItem key={e} value={e} onSelect={() => commit(e)}>
                    <CheckIcon className={cn('mr-2 size-4', value === e ? 'opacity-100' : 'opacity-0')} />
                    {e}
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
