import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ChevronDownIcon, ChevronUpIcon, XIcon } from 'lucide-react'
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

const DEFAULT_STEPS = ['page_view', 'view_item', 'cart_add', 'begin_checkout', 'order_placed']
const nfmt = (n: number) => n.toLocaleString()
const pct = (n: number, d: number) => (d > 0 ? Math.round((n / d) * 100) : 0)

export function SynapseFunnel() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [steps, setSteps] = useState<string[]>(DEFAULT_STEPS)

  // Available event names to build steps from = whatever this channel has recorded
  const { data: ov, error: ovError } = useQuery(SynapseOverviewDocument, {
    variables: { channelId, sinceDays: 30 },
    skip: !isChannel,
    fetchPolicy: 'cache-and-network',
  })
  const available = (ov?.synapseOverview.topEvents ?? []).map((e) => e.name)

  const { data, loading, error: funnelError } = useQuery(SynapseFunnelDocument, {
    variables: { channelId, steps, sinceDays: 30 },
    skip: !isChannel || steps.length < 2,
    fetchPolicy: 'cache-and-network',
  })
  const error = ovError ?? funnelError
  const result = data?.synapseFunnel.steps ?? []
  const top = result[0]?.count ?? 0

  const add = (e: string) => e && setSteps((s) => [...s, e])
  const remove = (i: number) => setSteps((s) => s.filter((_, k) => k !== i))
  const move = (i: number, d: -1 | 1) =>
    setSteps((s) => {
      const j = i + d
      if (j < 0 || j >= s.length) return s
      const next = [...s]
      ;[next[i], next[j]] = [next[j], next[i]]
      return next
    })

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Funnel"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Ordered conversion - how many people complete each step in sequence. People are resolved across their
            anonymous + signed-in activity. Last 30 days.
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
                {steps.map((s, i) => (
                  <div key={`${s}-${i}`} className="flex items-center gap-2 rounded border px-3 py-2 text-sm">
                    <span className="w-5 shrink-0 text-center text-xs font-semibold text-muted-foreground">{i + 1}</span>
                    <span className="flex-1 font-medium">{eventLabel(s)}</span>
                    <span className="text-xs text-muted-foreground">{s}</span>
                    <button type="button" onClick={() => move(i, -1)} disabled={i === 0} className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30">
                      <ChevronUpIcon className="size-4" />
                    </button>
                    <button type="button" onClick={() => move(i, 1)} disabled={i === steps.length - 1} className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30">
                      <ChevronDownIcon className="size-4" />
                    </button>
                    <button type="button" onClick={() => remove(i)} className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground">
                      <XIcon className="size-4" />
                    </button>
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
                            {nfmt(s.count)}
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
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
