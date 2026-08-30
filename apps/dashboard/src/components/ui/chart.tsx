import * as React from 'react'
import { ResponsiveContainer } from 'recharts'

import { cn } from '@/lib/utils'

type ChartTooltipItem = { name?: React.ReactNode; value?: number | string; color?: string; dataKey?: string | number }

// Theme-aware drop-in for recharts' default tooltip, which inherits no explicit colours and
// renders light-on-white under dark themes - built from the popover tokens so it stays legible
export function ChartTooltipContent({
  active,
  payload,
  label,
  labelFormatter,
  valueFormatter,
  hideLabel,
}: {
  active?: boolean
  payload?: ChartTooltipItem[]
  label?: string | number
  labelFormatter?: (label: string | number) => React.ReactNode
  valueFormatter?: (value: number | string | undefined, name?: React.ReactNode) => React.ReactNode
  hideLabel?: boolean
}) {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-md border bg-popover px-2.5 py-1.5 text-xs shadow-md">
      {!hideLabel && label != null && label !== '' && (
        <p className="mb-1 font-medium text-popover-foreground">{labelFormatter ? labelFormatter(label) : String(label)}</p>
      )}
      <div className="grid gap-0.5">
        {payload.map((p, i) => (
          <div key={i} className="flex items-center gap-2">
            {p.color && <span className="size-2 shrink-0 rounded-[2px]" style={{ background: p.color }} aria-hidden />}
            {p.name != null && p.name !== '' && <span className="text-muted-foreground">{p.name}</span>}
            <span className="ml-auto font-medium tabular-nums text-popover-foreground">
              {valueFormatter ? valueFormatter(p.value, p.name) : typeof p.value === 'number' ? p.value.toLocaleString() : p.value}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

// A deliberately small shell - a responsive fixed-height box recharts fills - Compose recharts primitives as the single child and theme
// axes and series with the design tokens at the call site. recharts v3 is used directly rather than wrapped, on purpose
export function ChartContainer({
  className,
  height = 280,
  children,
}: {
  className?: string
  height?: number
  children: React.ReactElement
}) {
  return (
    <div className={cn('w-full', className)} style={{ height }}>
      <ResponsiveContainer width="100%" height="100%">
        {children}
      </ResponsiveContainer>
    </div>
  )
}
