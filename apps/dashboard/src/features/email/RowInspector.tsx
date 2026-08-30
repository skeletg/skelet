import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import type { Column, EmailBlock } from './blocks'

// RowInspector edits a row's column structure: how many columns, and each column's relative width (a weight - e.g. 1 + 2 renders 33% / 66%)
// - Adding columns appends empty ones; removing drops the trailing columns (and any blocks in them)
export function RowInspector({
  block,
  onChange,
}: {
  block: EmailBlock
  onChange: (b: EmailBlock) => void
}) {
  const columns = block.columns ?? []

  function setCount(n: number) {
    const next: Column[] = []
    for (let i = 0; i < n; i++) next.push(columns[i] ?? { width: 1, blocks: [] })
    onChange({ ...block, columns: next })
  }

  function setWidth(i: number, width: number) {
    onChange({
      ...block,
      columns: columns.map((c, ci) => (ci === i ? { ...c, width } : c)),
    })
  }

  return (
    <div className="grid gap-5">
      <div className="grid gap-1.5">
        <Label className="text-xs">Columns</Label>
        <div className="flex gap-1">
          {[1, 2, 3, 4].map((n) => (
            <button
              key={n}
              type="button"
              onClick={() => setCount(n)}
              className={cn(
                'h-8 flex-1 rounded border text-sm transition-colors',
                columns.length === n
                  ? 'border-primary bg-primary/10 text-primary'
                  : 'bg-background text-muted-foreground hover:bg-accent',
              )}
            >
              {n}
            </button>
          ))}
        </div>
      </div>

      <div className="grid gap-2 border-t pt-4">
        <Label className="text-xs">Column widths (relative weight)</Label>
        {columns.map((c, i) => (
          <div key={i} className="flex items-center gap-2">
            <span className="w-16 text-xs text-muted-foreground">Column {i + 1}</span>
            <Input
              type="number"
              min={1}
              max={12}
              value={c.width && c.width > 0 ? c.width : 1}
              onChange={(e) => setWidth(i, Math.max(1, Math.min(12, Number(e.target.value || 1))))}
              className="h-8 text-xs"
            />
          </div>
        ))}
        <p className="text-xs text-muted-foreground">
          Drop blocks into each column on the canvas. Widths are proportional - e.g. 1 and 2 render
          one-third / two-thirds.
        </p>
      </div>
    </div>
  )
}
