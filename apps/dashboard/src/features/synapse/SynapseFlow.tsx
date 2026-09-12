import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ChevronRightIcon, PlayIcon, GitForkIcon } from 'lucide-react'
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
import { type SynapseFlowQuery, SynapseFlowDocument, SynapseOverviewDocument } from './Synapse.gql'

type FlowNode = { id: string; parentId: string; event: string; depth: number; count: number }
const nfmt = (n: number) => n.toLocaleString()
const TOP = 12 // show the busiest next steps; rarer ones fold into "show more"

export function SynapseFlow() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [focus, setFocus] = useState('page_view')

  const { data: ov, error: ovError } = useQuery(SynapseOverviewDocument, { variables: { channelId, sinceDays: 30 }, skip: !isChannel, fetchPolicy: 'cache-and-network' })
  // Drop the opaque "custom" bucket as a starting point - the flow now keys custom
  // events by their real name, so a bare "custom" focus would match nothing
  const events = (ov?.synapseOverview.topEvents ?? []).map((e) => e.name).filter((e) => e !== 'custom')

  const { data, loading, error: flowError } = useQuery(SynapseFlowDocument, {
    variables: { channelId, event: focus, sinceDays: 30, depth: 4 },
    skip: !isChannel || !focus,
    fetchPolicy: 'cache-and-network',
  })
  const flow = data?.synapseFlow
  const error = ovError ?? flowError

  const byParent = useMemo(() => {
    const m: Record<string, FlowNode[]> = {}
    for (const n of flow?.nodes ?? []) (m[n.parentId] ??= []).push(n)
    return m
  }, [flow])

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Flow"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            What people do next, one step at a time - click a step to follow the path deeper. People resolved across
            anonymous + signed-in activity. Last 30 days.
          </span>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load flow</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Flows are per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection
            title="Path from"
            description="Each bar is the share of the step in view that went there next. Click a step with an arrow to see what came after it."
            trailing={
              <select
                value={focus}
                onChange={(e) => setFocus(e.target.value)}
                aria-label="Starting event"
                className="rounded-md border bg-background px-2.5 py-1.5 text-sm font-medium"
              >
                {(events.includes(focus) ? events : [focus, ...events]).map((e) => (
                  <option key={e} value={e}>
                    {eventLabel(e)}
                  </option>
                ))}
              </select>
            }
          >
            {!flow ? (
              <p className="py-6 text-sm text-muted-foreground">{loading ? 'Computing paths…' : 'No data.'}</p>
            ) : flow.cohort === 0 ? (
              <p className="py-6 text-sm text-muted-foreground">No one did “{eventLabel(flow.focus)}” in the last 30 days - pick another starting event.</p>
            ) : (
              // Keyed remount: switching the channel (or the focus event) resets the drill trail, so a trail from
              // another cohort can never mix with fresh data (mixed counts, >100% shares)
              <FlowDrilldown key={`${channelId}:${focus}`} flow={flow} byParent={byParent} />
            )}
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// FlowDrilldown is the interactive explorer for one loaded flow: it owns the
// drill trail (the sequence of clicked steps after the focus event; empty =
// looking at the focus event's immediate next steps) - The parent remounts it
// via key when the channel or focus changes, which resets the trail
function FlowDrilldown({
  flow,
  byParent,
}: {
  flow: NonNullable<SynapseFlowQuery['synapseFlow']>
  byParent: Record<string, FlowNode[]>
}) {
  const [trail, setTrail] = useState<FlowNode[]>([])

  // The step currently in view: the last drilled node, else the focus event
  const current = trail[trail.length - 1]
  const currentId = current ? current.id : flow.focus
  const currentCount = current ? current.count : flow.cohort
  const currentLabel = eventLabel(current ? current.event : flow.focus)

  return (
    <div className="space-y-3">
      <Breadcrumb focus={flow.focus} trail={trail} onJump={(i) => setTrail(trail.slice(0, i))} />

      {/* The cohort currently in view */}
      <div className="flex items-center gap-3 rounded-lg border bg-primary/5 p-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-full bg-primary text-primary-foreground">
          <PlayIcon className="size-4 fill-current" />
        </span>
        <div className="min-w-0">
          <div className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
            {trail.length === 0 ? 'Starting point' : `Step ${trail.length + 1}`}
          </div>
          <div className="truncate font-medium">{currentLabel}</div>
        </div>
        <div className="ml-auto text-right">
          <div className="text-2xl font-semibold leading-none tabular-nums">{nfmt(currentCount)}</div>
          <div className="mt-0.5 text-xs text-muted-foreground">{trail.length === 0 ? 'people' : 'reached here'}</div>
        </div>
      </div>

      {/* Headline comparison - only at the root cohort */}
      {trail.length === 0 ? (
        <FlowSummary focus={flow.focus} cohort={flow.cohort} continued={flow.continued} avgSteps={flow.avgSteps} />
      ) : null}

      <NextSteps
        key={currentId}
        steps={byParent[currentId] ?? []}
        cohort={currentCount}
        byParent={byParent}
        onDrill={(n) => setTrail([...trail, n])}
      />

      {/* The most common complete journeys from the starting event */}
      {trail.length === 0 && flow.topPaths.length > 0 ? (
        <div className="mt-6 border-t pt-4">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <GitForkIcon className="size-4 text-muted-foreground" />
            <h3 className="text-sm font-semibold">Most common journeys</h3>
            <span className="text-xs text-muted-foreground">complete paths from {eventLabel(flow.focus)}, by how many took them</span>
          </div>
          <TopPaths focus={flow.focus} paths={flow.topPaths} cohort={flow.cohort} />
        </div>
      ) : null}
    </div>
  )
}

// FlowSummary is the at-a-glance comparison for the starting cohort: what share
// continued vs left after the focus event, and how deep the average journey ran
function FlowSummary({ focus, cohort, continued, avgSteps }: { focus: string; cohort: number; continued: number; avgSteps: number }) {
  const dropped = Math.max(0, cohort - continued)
  const contPct = cohort > 0 ? Math.round((continued / cohort) * 100) : 0
  const dropPct = cohort > 0 ? Math.round((dropped / cohort) * 100) : 0
  return (
    <div className="rounded-lg border p-3">
      <div className="mb-1.5 flex items-center justify-between text-xs">
        <span className="font-medium text-foreground">{contPct}% continued</span>
        <span className="text-muted-foreground">{dropPct}% left after {eventLabel(focus)}</span>
      </div>
      <div className="flex h-2 overflow-hidden rounded-full bg-muted" role="img" aria-label={`${contPct}% continued, ${dropPct}% left`}>
        <span className="bg-primary" style={{ width: `${contPct}%` }} />
      </div>
      <div className="mt-2 text-xs text-muted-foreground">
        {nfmt(continued)} of {nfmt(cohort)} took a next step{continued > 0 ? ` · averaging ${avgSteps.toFixed(1)} steps deep` : ''}
      </div>
    </div>
  )
}

// TopPaths lists the most common complete journeys as event chip sequences with a
// share bar - the "what's the dominant route" view to complement the drill-down
function TopPaths({ focus, paths, cohort }: { focus: string; paths: { events: string[]; count: number }[]; cohort: number }) {
  const max = Math.max(1, ...paths.map((p) => p.count))
  return (
    <div className="space-y-1.5">
      {paths.map((p, i) => {
        const pct = cohort > 0 ? Math.round((p.count / cohort) * 100) : 0
        return (
          <div key={i} className="rounded-md border p-2">
            <div className="mb-1.5 flex items-start justify-between gap-3">
              <div className="flex flex-wrap items-center gap-1">
                <span className="rounded bg-primary/10 px-1.5 py-0.5 text-xs font-medium text-primary">{eventLabel(focus)}</span>
                {p.events.map((e, j) => (
                  <span key={j} className="flex items-center gap-1">
                    <ChevronRightIcon className="size-3 shrink-0 text-muted-foreground/50" />
                    <span className="rounded bg-muted px-1.5 py-0.5 text-xs">{eventLabel(e)}</span>
                  </span>
                ))}
              </div>
              <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                {nfmt(p.count)}
                <span className="ml-1.5 font-semibold text-foreground">{pct}%</span>
              </span>
            </div>
            <span className="block h-1 overflow-hidden rounded-full bg-muted">
              <span className="block h-full rounded-full bg-primary/60" style={{ width: `${Math.max(2, (p.count / max) * 100)}%` }} />
            </span>
          </div>
        )
      })}
    </div>
  )
}

// Breadcrumb shows the focus event + each drilled step; any crumb jumps back to it
function Breadcrumb({ focus, trail, onJump }: { focus: string; trail: FlowNode[]; onJump: (index: number) => void }) {
  const crumbs = [{ label: eventLabel(focus) }, ...trail.map((n) => ({ label: eventLabel(n.event) }))]
  return (
    <nav className="flex flex-wrap items-center gap-1 text-sm" aria-label="Path">
      {crumbs.map((c, i) => {
        const isLast = i === crumbs.length - 1
        return (
          <span key={i} className="flex items-center gap-1">
            {i > 0 ? <ChevronRightIcon className="size-3.5 text-muted-foreground/60" /> : null}
            {isLast ? (
              <span className="font-medium">{c.label}</span>
            ) : (
              <button type="button" onClick={() => onJump(i)} className="rounded text-muted-foreground hover:text-foreground hover:underline">
                {c.label}
              </button>
            )}
          </span>
        )
      })}
    </nav>
  )
}

// NextSteps renders one level: the cohort's next events as ranked funnel bars (width = share of the cohort), a "show
// more" fold for the long tail, and a single drop-off row for everyone who took no tracked next step
function NextSteps({ steps, cohort, byParent, onDrill }: { steps: FlowNode[]; cohort: number; byParent: Record<string, FlowNode[]>; onDrill: (n: FlowNode) => void }) {
  const [expanded, setExpanded] = useState(false)
  const sorted = useMemo(() => steps.slice().sort((a, b) => b.count - a.count), [steps])
  const continued = sorted.reduce((s, n) => s + n.count, 0)
  const dropped = Math.max(0, cohort - continued)
  const dropPct = cohort > 0 ? Math.round((dropped / cohort) * 100) : 0

  if (sorted.length === 0) {
    return (
      <p className="rounded-lg border border-dashed px-3 py-4 text-sm text-muted-foreground">
        Nobody took a tracked step after this - the journey ends here.
      </p>
    )
  }

  const shown = expanded ? sorted : sorted.slice(0, TOP)
  const hiddenCount = sorted.length - shown.length

  return (
    <div className="space-y-1.5">
      {shown.map((n) => {
        const pct = cohort > 0 ? Math.round((n.count / cohort) * 100) : 0
        const drillable = (byParent[n.id]?.length ?? 0) > 0
        const Tag = drillable ? 'button' : 'div'
        return (
          <Tag
            key={n.id}
            {...(drillable ? { type: 'button' as const, onClick: () => onDrill(n) } : {})}
            className={cn(
              'relative block h-10 w-full overflow-hidden rounded-md border text-left',
              drillable ? 'cursor-pointer hover:border-primary/50 hover:shadow-sm' : 'cursor-default',
            )}
          >
            <span className="absolute inset-y-0 left-0 bg-primary/15" style={{ width: `${Math.max(1.5, pct)}%` }} aria-hidden />
            <span className="absolute inset-y-0 left-0 w-0.5 bg-primary/70" aria-hidden />
            <span className="relative flex h-full items-center gap-2 px-3">
              <span className="truncate text-sm font-medium">{eventLabel(n.event)}</span>
              <span className="ml-auto shrink-0 text-xs tabular-nums text-muted-foreground">
                {nfmt(n.count)}
                <span className="ml-1.5 font-semibold text-foreground">{pct}%</span>
              </span>
              {drillable ? <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" /> : <span className="w-4 shrink-0" aria-hidden />}
            </span>
          </Tag>
        )
      })}

      {hiddenCount > 0 ? (
        <button type="button" onClick={() => setExpanded(true)} className="px-1 py-1 text-xs font-medium text-primary hover:underline">
          Show {hiddenCount} more {hiddenCount === 1 ? 'step' : 'steps'}
        </button>
      ) : null}

      {dropped > 0 ? (
        <div className="mt-1 flex items-center gap-2 rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground">
          <span className="size-1.5 rounded-full bg-muted-foreground/40" aria-hidden />
          <span>No further activity</span>
          <span className="ml-auto tabular-nums">
            {nfmt(dropped)}
            <span className="ml-1.5 font-medium">{dropPct}%</span>
          </span>
        </div>
      ) : null}
    </div>
  )
}
