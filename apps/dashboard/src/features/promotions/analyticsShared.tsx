import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { ACCENT, fmtDay, formatCents, type DailyPoint } from './promoFormat'

// Redemptions-over-time area chart shared by the Promotions hub (full-width) and the per-promotion page (compact) -
// Height + gradient id are configurable so two instances on one page don't collide
export function RedemptionsTrend({
  daily,
  height = 224,
  gradientId = 'promoTrend',
}: {
  daily: DailyPoint[]
  height?: number
  gradientId?: string
}) {
  if (daily.length === 0) {
    return (
      <p className="py-10 text-center text-sm text-muted-foreground">
        No redemptions in this range yet.
      </p>
    )
  }
  return (
    <div style={{ height }} className="w-full">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={daily} margin={{ top: 8, right: 8, bottom: 0, left: -16 }}>
          <defs>
            <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={ACCENT} stopOpacity={0.3} />
              <stop offset="100%" stopColor={ACCENT} stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
          <XAxis
            dataKey="date"
            tick={{ fontSize: 11 }}
            tickFormatter={fmtDay}
            minTickGap={28}
            tickMargin={6}
          />
          <YAxis tick={{ fontSize: 11 }} allowDecimals={false} width={40} />
          <Tooltip content={<TrendTooltip />} cursor={{ stroke: ACCENT, strokeOpacity: 0.25 }} />
          <Area
            type="monotone"
            dataKey="redemptionCount"
            stroke={ACCENT}
            strokeWidth={2}
            fill={`url(#${gradientId})`}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

function TrendTooltip({
  active,
  label,
  payload,
}: {
  active?: boolean
  label?: string
  payload?: Array<{ payload: { redemptionCount: number; discountTotalCents: number } }>
}) {
  if (!active || !payload?.length) return null
  const p = payload[0].payload
  return (
    <div className="rounded border bg-background px-2.5 py-1.5 text-xs shadow">
      <div className="font-medium">{label ? fmtDay(label) : ''}</div>
      <div className="text-muted-foreground">
        {p.redemptionCount.toLocaleString()} redemptions · {formatCents(p.discountTotalCents)}
      </div>
    </div>
  )
}
