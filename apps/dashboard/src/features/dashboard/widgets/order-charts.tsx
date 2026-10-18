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
import { CHART_ACCENT } from '@/lib/chart-colors'
import { OrderStatusBadge } from '@/features/orders/OrderStatusBadge'
import { SALES_DAYS, useSalesReport } from './sales'
import { Empty, Panel, WidgetSection } from './primitives'
import { formatNumber } from '@/lib/format'

const dayLabel = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' })

// Volume and lifecycle, both bucketed by the server - never the client reducing over every order
export function OrderChartsWidget() {
  const { loading, error, series, statusBreakdown } = useSalesReport()
  const points = series.map((p) => ({ label: dayLabel.format(new Date(p.period)), count: p.orders }))
  const total = statusBreakdown.reduce((sum, s) => sum + s.orderCount, 0) || 1
  const rows = [...statusBreakdown]
    .sort((a, b) => b.orderCount - a.orderCount)
    .map((s) => ({ ...s, share: Math.round((s.orderCount / total) * 100) }))

  return (
    <WidgetSection
      title="Orders"
      description={`Incoming volume and where the last ${SALES_DAYS} days of orders sit in their lifecycle.`}
      to="/orders"
      loading={loading && points.length === 0}
      error={error}
    >
      <div className="grid gap-4 lg:grid-cols-2">
        <Panel title={`Order volume - last ${SALES_DAYS} days`}>
          {points.length === 0 ? (
            <Empty>No orders yet.</Empty>
          ) : (
            <div className="h-56">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={points} margin={{ top: 8, right: 12, bottom: 0, left: -16 }}>
                  <defs>
                    <linearGradient id="home-vol" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor={CHART_ACCENT} stopOpacity={0.35} />
                      <stop offset="100%" stopColor={CHART_ACCENT} stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                  <XAxis dataKey="label" tick={{ fontSize: 11 }} interval="preserveStartEnd" minTickGap={20} />
                  <YAxis tick={{ fontSize: 11 }} allowDecimals={false} width={28} />
                  <Tooltip content={<ChartTooltipContent />} />
                  <Area type="monotone" dataKey="count" name="Orders" stroke={CHART_ACCENT} strokeWidth={2} fill="url(#home-vol)" />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          )}
        </Panel>

        <Panel title="Status mix">
          {rows.length === 0 ? (
            <Empty>No orders yet.</Empty>
          ) : (
            <div className="space-y-2.5 py-1">
              {rows.map((row) => (
                <div key={row.status} className="grid gap-1.5">
                  <div className="flex items-center justify-between gap-3">
                    <OrderStatusBadge status={row.status} />
                    <span className="text-xs tabular-nums text-muted-foreground">
                      {formatNumber(row.orderCount)} · {row.share}%
                    </span>
                  </div>
                  <span className="h-2 overflow-hidden rounded-full bg-muted">
                    <span className="block h-full rounded-full bg-primary" style={{ width: `${Math.max(row.share, 3)}%` }} />
                  </span>
                </div>
              ))}
            </div>
          )}
        </Panel>
      </div>
    </WidgetSection>
  )
}
