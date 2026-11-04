import { PERIODS } from './report'

// PeriodPicker is the window a board, or a report page, reads over
export function PeriodPicker({ value, onChange }: { value: number; onChange: (days: number) => void }) {
  return (
    <label className="flex items-center gap-2 text-sm">
      <span className="text-muted-foreground">Period</span>
      <select value={value} onChange={(e) => onChange(Number(e.target.value))} className="rounded border bg-background px-2 py-1.5 text-sm">
        {PERIODS.map((d) => (
          <option key={d} value={d}>
            Last {d} days
          </option>
        ))}
      </select>
    </label>
  )
}
