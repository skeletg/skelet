import { useQuery } from '@apollo/client/react'
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { ChartTooltipContent } from '@/components/ui/chart'
import { useConfigScope } from '@/lib/config-scope'
import { LensOverviewDocument } from '@/features/lens/Lens.gql'
import { NeedsChannel, Tile, WidgetSection } from './primitives'

const ACCENT = '#6366f1'
const DAYS = 30
const dayLabel = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' })

export function TrafficWidget() {
  const { effectiveChannelId } = useConfigScope()
  const { data, loading, error } = useQuery(LensOverviewDocument, {
    variables: { channelId: effectiveChannelId ?? '', sinceDays: DAYS },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })
  const overview = data?.lensOverview
  const points = (overview?.daily ?? []).map((d) => ({
    label: dayLabel.format(new Date(d.date)),
    views: d.views,
  }))

  return (
    <WidgetSection
      title="Traffic"
      description={`Storefront page views over the last ${DAYS} days.`}
      to="/lens"
      loading={!!effectiveChannelId && loading && !overview}
      error={error ?? null}
    >
      {!effectiveChannelId ? (
        <NeedsChannel what="traffic" />
      ) : (
        <div className="grid gap-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <Tile label="Page views" value={(overview?.views ?? 0).toLocaleString()} sub={`last ${DAYS} days`} />
            <Tile label="Visitors" value={(overview?.visitors ?? 0).toLocaleString()} sub="distinct, excluding bots" />
            <Tile
              label="Top page"
              value={overview?.topPages[0]?.pageKey ?? '-'}
              sub={overview?.topPages[0] ? `${overview.topPages[0].views.toLocaleString()} views` : undefined}
            />
          </div>
          {points.length > 0 && (
            <div className="h-48 rounded border p-3">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={points} margin={{ top: 8, right: 12, bottom: 0, left: -16 }}>
                  <defs>
                    <linearGradient id="home-traffic" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor={ACCENT} stopOpacity={0.35} />
                      <stop offset="100%" stopColor={ACCENT} stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                  <XAxis dataKey="label" tick={{ fontSize: 11 }} interval="preserveStartEnd" minTickGap={24} />
                  <YAxis tick={{ fontSize: 11 }} allowDecimals={false} width={36} />
                  <Tooltip content={<ChartTooltipContent />} />
                  <Area type="monotone" dataKey="views" name="Views" stroke={ACCENT} strokeWidth={2} fill="url(#home-traffic)" />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          )}
        </div>
      )}
    </WidgetSection>
  )
}
