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
import { useConfigScope } from '@/lib/config-scope'
import { eventLabel } from './eventLabel'
import { SynapseOverviewDocument, SynapseStickinessDocument } from './Synapse.gql'

const nfmt = (n: number) => n.toLocaleString()
function fmtDay(v: string): string {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}
// Common return cadences - a store picks what "coming back" means for its catalog
const WINDOWS = [7, 14, 30, 60, 90, 180, 365]
const windowLabel = (d: number) => (d % 365 === 0 ? `${d / 365}y` : d % 30 === 0 ? `${d / 30}mo` : d % 7 === 0 ? `${d / 7}wk` : `${d}d`)

export function SynapseStickiness() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [event, setEvent] = useState('') // '' = any activity
  const [win, setWin] = useState<number | null>(null) // null = channel-configured default

  const { data: ov, error: ovError } = useQuery(SynapseOverviewDocument, { variables: { channelId, sinceDays: 30 }, skip: !isChannel, fetchPolicy: 'cache-and-network' })
  const events = (ov?.synapseOverview.topEvents ?? []).map((e) => e.name)

  const { data, loading, error: stickError } = useQuery(SynapseStickinessDocument, {
    variables: { channelId, event, sinceDays: 30, windowDays: win },
    skip: !isChannel,
    fetchPolicy: 'cache-and-network',
  })
  const s = data?.synapseStickiness
  const error = ovError ?? stickError
  const window = win ?? s?.windowDays ?? 30
  const max = Math.max(1, ...(s?.days ?? []).map((d) => d.count))

  const returning = s?.returning ?? 0
  const activeNow = s?.activeNow ?? 0
  const fresh = Math.max(0, activeNow - returning) // new or reactivated this window
  const returnPct = Math.round(s?.returnRate ?? 0)
  const retShare = activeNow > 0 ? Math.round((returning / activeNow) * 100) : 0

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Engagement"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Do people come back? The return rate compares one window to the next - set the window to your store’s buying
            cadence. People resolved across anonymous + signed-in activity.
          </span>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load engagement</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Engagement is per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection
              title="Return rate"
              description="Of the people active in the previous window, how many came back in the current one. The window should match how often you’d expect a repeat visit - weekly for consumables, much longer for big-ticket items."
              trailing={
                <div className="flex flex-wrap items-end gap-3">
                  <label className="flex flex-col gap-1 text-xs text-muted-foreground">
                    Active means doing
                    <select value={event} onChange={(e) => setEvent(e.target.value)} className="w-48 rounded-md border bg-background px-2 py-1.5 text-sm">
                      <option value="">Any activity</option>
                      {events.map((e) => (
                        <option key={e} value={e}>
                          {eventLabel(e)}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="flex flex-col gap-1 text-xs text-muted-foreground">
                    Return window
                    <select
                      value={String(window)}
                      onChange={(e) => setWin(Number(e.target.value))}
                      className="w-28 rounded-md border bg-background px-2 py-1.5 text-sm"
                    >
                      {(WINDOWS.includes(window) ? WINDOWS : [window, ...WINDOWS]).map((d) => (
                        <option key={d} value={d}>
                          {windowLabel(d)} ({d}d)
                        </option>
                      ))}
                    </select>
                  </label>
                </div>
              }
            >
              {!s ? (
                <p className="py-6 text-sm text-muted-foreground">{loading ? 'Computing…' : 'No data.'}</p>
              ) : s.priorActive === 0 ? (
                <p className="py-6 text-sm text-muted-foreground">
                  No activity in the previous {window}-day window to measure returns against yet - widen the window or wait for more history.
                </p>
              ) : (
                <div className="grid gap-4 lg:grid-cols-[260px_1fr]">
                  {/* Headline return rate */}
                  <div className="rounded-lg border bg-primary/5 p-4">
                    <div className="text-5xl font-semibold leading-none tabular-nums text-primary">{returnPct}%</div>
                    <div className="mt-2 text-sm font-medium">came back</div>
                    <div className="mt-1 text-xs text-muted-foreground">
                      {nfmt(returning)} of {nfmt(s.priorActive)} people active in the prior {window} days returned in the last {window}.
                    </div>
                  </div>

                  {/* New vs returning split of the current window */}
                  <div className="rounded-lg border p-4">
                    <div className="mb-2 flex items-baseline justify-between">
                      <span className="text-sm font-medium">Active in the last {window} days</span>
                      <span className="text-lg font-semibold tabular-nums">{nfmt(activeNow)}</span>
                    </div>
                    <div className="flex h-2.5 overflow-hidden rounded-full bg-muted" role="img" aria-label={`${retShare}% returning, ${100 - retShare}% new or reactivated`}>
                      <span className="bg-primary" style={{ width: `${retShare}%` }} />
                      <span className="bg-sky-400" style={{ width: `${100 - retShare}%` }} />
                    </div>
                    <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
                      <div className="flex items-center gap-2">
                        <span className="size-2.5 rounded-full bg-primary" />
                        <span className="text-muted-foreground">Returning</span>
                        <span className="ml-auto tabular-nums font-medium">{nfmt(returning)}</span>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="size-2.5 rounded-full bg-sky-400" />
                        <span className="text-muted-foreground">New or reactivated</span>
                        <span className="ml-auto tabular-nums font-medium">{nfmt(fresh)}</span>
                      </div>
                    </div>
                    <p className="mt-3 border-t pt-2 text-xs text-muted-foreground">
                      For reference: {nfmt(s.wau)} active in the last 7 days · {nfmt(s.mau)} in the last 30.
                    </p>
                  </div>
                </div>
              )}
            </DetailPageSection>

            {s && s.days.length > 0 && (
              <DetailPageSection title="Active people per day" description="Distinct people with any tracked activity each day (trend context for the rate above).">
                <div className="rounded border p-3">
                  <div className="flex h-36 items-end gap-0.5">
                    {s.days.map((d) => (
                      <div
                        key={d.day}
                        title={`${fmtDay(d.day)}: ${nfmt(d.count)}`}
                        className="flex-1 rounded-t bg-primary/70 hover:bg-primary"
                        style={{ height: `${Math.max(2, Math.round((d.count / max) * 100))}%` }}
                      />
                    ))}
                  </div>
                  <div className="mt-1.5 flex justify-between text-[11px] text-muted-foreground">
                    <span>{fmtDay(s.days[0].day)}</span>
                    <span>{fmtDay(s.days[s.days.length - 1].day)}</span>
                  </div>
                </div>
              </DetailPageSection>
            )}
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
