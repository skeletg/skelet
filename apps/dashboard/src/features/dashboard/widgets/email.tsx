import { useQuery } from '@apollo/client/react'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'
import { PulseEngagementDocument } from '@/features/pulse/Pulse.signals.gql'
import { NeedsChannel, Tile, WidgetSection } from './primitives'

const DAYS = 30

export function EmailWidget() {
  const { effectiveChannelId } = useConfigScope()
  const { data, loading, error } = useQuery(PulseEngagementDocument, {
    variables: { channelId: effectiveChannelId ?? '', sinceDays: DAYS },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const e = data?.pulseEngagement
  // revenue is per-currency; the primary one is enough for a dashboard tile
  const revenue = e?.revenue[0]

  return (
    <WidgetSection
      title="Email engagement"
      description={`Opens, clicks and what they earned over the last ${DAYS} days.`}
      to="/pulse"
      loading={!!effectiveChannelId && loading && !e}
      error={error ?? null}
    >
      {!effectiveChannelId ? (
        <NeedsChannel what="email engagement" />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {/* human opens, not raw - prefetching proxies inflate the raw number */}
          <Tile label="Opens" value={(e?.opensHuman ?? 0).toLocaleString()} sub="excluding prefetches" />
          <Tile label="Clicks" value={(e?.clicks ?? 0).toLocaleString()} />
          <Tile label="Conversions" value={(e?.conversions ?? 0).toLocaleString()} />
          <Tile
            label="Attributed revenue"
            value={revenue ? formatCents(revenue.cents, revenue.currency) : '-'}
            sub={revenue ? `${revenue.orders.toLocaleString()} orders` : undefined}
          />
        </div>
      )}
    </WidgetSection>
  )
}
