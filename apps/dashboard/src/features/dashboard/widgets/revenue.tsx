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
import { formatCents } from '@/lib/format'
import { SALES_DAYS, useSalesReport } from './sales'
import { Empty, Tile, WidgetSection } from './primitives'

const ACCENT = '#6366f1'
const dayLabel = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' })

// The money section: reporting-grade figures (margin and refunds included, which the tiles can't
// express) plus the trend, all from one server-side aggregate
export function RevenueWidget() {
  const { loading, error, currency, summary, multiCurrency, series } = useSalesReport()
  const money = (cents: number | undefined) => (currency ? formatCents(cents ?? 0, currency) : '-')
  const points = series.map((p) => ({
    label: dayLabel.format(new Date(p.period)),
    revenue: p.revenue,
    orders: p.orders,
  }))

  return (
    <WidgetSection
      title="Revenue"
      description={
        multiCurrency
          ? `Last ${SALES_DAYS} days in ${currency}, the store's primary currency. Cancelled orders excluded.`
          : `Last ${SALES_DAYS} days. Cancelled orders excluded.`
      }
      loading={loading && !summary}
      error={error}
      empty={!loading && !summary}
    >
      <div className="grid gap-4">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Tile label="Gross revenue" value={money(summary?.grossRevenue)} sub={`${(summary?.orderCount ?? 0).toLocaleString()} orders`} />
          <Tile label="Avg order value" value={money(summary?.averageOrderValue)} />
          <Tile label="Margin" value={money(summary?.marginTotal)} sub={`cost ${money(summary?.costTotal)}`} />
          <Tile label="Refunds" value={money(summary?.refundedAmount)} sub={`${summary?.refundCount ?? 0} refunded`} />
        </div>
        {points.length === 0 ? (
          <Empty>No sales in this window.</Empty>
        ) : (
          <div className="h-56 rounded border p-3">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={points} margin={{ top: 8, right: 12, bottom: 0, left: -8 }}>
                <defs>
                  <linearGradient id="home-rev" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={ACCENT} stopOpacity={0.35} />
                    <stop offset="100%" stopColor={ACCENT} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" className="stroke-muted" vertical={false} />
                <XAxis dataKey="label" tick={{ fontSize: 11 }} interval="preserveStartEnd" minTickGap={20} />
                <YAxis
                  tick={{ fontSize: 11 }}
                  width={54}
                  tickFormatter={(v: number) => formatCents(v, currency)}
                />
                <Tooltip content={<ChartTooltipContent />} formatter={(v) => formatCents(Number(v), currency)} />
                <Area type="monotone" dataKey="revenue" name="Revenue" stroke={ACCENT} strokeWidth={2} fill="url(#home-rev)" />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>
    </WidgetSection>
  )
}
