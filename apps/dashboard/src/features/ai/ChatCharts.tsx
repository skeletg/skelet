import { Bar, BarChart, CartesianGrid, Legend, Line, LineChart, Tooltip, XAxis, YAxis } from 'recharts'
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'
import { CHART_ACCENT } from '@/lib/chart-colors'

// The assistant's chart widgets, split from ChatMessage so recharts loads only when an answer draws one

export type BarDatum = { label?: string; value?: number }
type LinePoint = { x?: string | number; y?: number }
export type LineSeries = { label?: string; data?: LinePoint[] }

export function BarWidget({ title, data, unit }: { title?: string; data?: BarDatum[]; unit?: string }) {
  const rows = Array.isArray(data)
    ? data.filter((d): d is BarDatum => !!d && d.label != null && typeof d.value === 'number')
    : []
  if (!rows.length) return null
  const height = Math.max(96, rows.length * 34 + 24)
  const fmt = (v: number | string | undefined) => {
    const n = typeof v === 'number' ? v.toLocaleString() : String(v ?? '')
    if (unit === '$') return `$${n}`
    return unit ? `${n} ${unit}` : n
  }
  return (
    <div className="rounded-xl border bg-muted/30 p-3">
      {title ? <div className="mb-2 text-xs font-medium text-muted-foreground">{title}</div> : null}
      <ChartContainer height={height}>
        <BarChart data={rows} layout="vertical" margin={{ top: 0, right: 12, bottom: 0, left: 4 }}>
          <CartesianGrid horizontal={false} strokeDasharray="3 3" stroke="var(--color-border)" />
          <XAxis type="number" tick={{ fontSize: 11 }} allowDecimals={false} />
          <YAxis
            type="category"
            dataKey="label"
            width={96}
            tick={{ fontSize: 11 }}
            tickFormatter={(v: string) => (v.length > 16 ? `${v.slice(0, 15)}…` : v)}
          />
          <Tooltip cursor={{ fill: 'var(--color-muted)' }} content={<ChartTooltipContent valueFormatter={fmt} />} />
          <Bar dataKey="value" name={title || 'Value'} fill={CHART_ACCENT} radius={[0, 4, 4, 0]} barSize={14} />
        </BarChart>
      </ChartContainer>
    </div>
  )
}

// LineWidget renders one or two time-series as a line chart (trends, retention curves) - Series are merged into one
// row[] keyed by x (first-seen order), with each series as its own dataKey
export function LineWidget({ title, series, unit }: { title?: string; series?: LineSeries[]; unit?: string }) {
  const list = Array.isArray(series) ? series.filter((s) => s && Array.isArray(s.data) && s.data.length) : []
  if (!list.length) return null
  const rows: Record<string, number | string>[] = []
  const byX = new Map<string | number, Record<string, number | string>>()
  const keys = list.map((s, si) => s.label || `Series ${si + 1}`)
  list.forEach((s, si) => {
    const key = keys[si]
    const data = s.data ?? []
    data.forEach((pt) => {
      if (!pt || pt.x == null || typeof pt.y !== 'number') return
      let row = byX.get(pt.x)
      if (!row) {
        row = { x: pt.x }
        byX.set(pt.x, row)
        rows.push(row)
      }
      row[key] = pt.y
    })
  })
  if (!rows.length) return null
  const colors = [CHART_ACCENT, 'var(--color-muted-foreground)']
  const fmt = (v: number | string | undefined) => {
    const n = typeof v === 'number' ? v.toLocaleString() : String(v ?? '')
    if (unit === '$') return `$${n}`
    return unit ? `${n} ${unit}` : n
  }
  return (
    <div className="rounded-xl border bg-muted/30 p-3">
      {title ? <div className="mb-2 text-xs font-medium text-muted-foreground">{title}</div> : null}
      <ChartContainer height={200}>
        <LineChart data={rows} margin={{ top: 8, right: 12, bottom: 0, left: -8 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
          <XAxis dataKey="x" tick={{ fontSize: 11 }} interval="preserveStartEnd" minTickGap={24} />
          <YAxis tick={{ fontSize: 11 }} width={44} tickFormatter={(v: number) => v.toLocaleString()} />
          <Tooltip content={<ChartTooltipContent valueFormatter={fmt} />} />
          {keys.length > 1 ? <Legend wrapperStyle={{ fontSize: 11 }} /> : null}
          {keys.map((k, i) => (
            <Line key={k} type="monotone" dataKey={k} name={k} stroke={colors[i % colors.length]} strokeWidth={2} dot={false} />
          ))}
        </LineChart>
      </ChartContainer>
    </div>
  )
}
