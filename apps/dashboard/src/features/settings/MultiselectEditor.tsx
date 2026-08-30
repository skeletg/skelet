import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'

export function MultiselectEditor({
  options,
  value,
  onChange,
  disabled,
}: {
  options: { label: string; value: string }[]
  value: string
  onChange: (v: string) => void
  disabled?: boolean
}) {
  const selected = value ? value.split(',').filter(Boolean) : []

  function toggle(val: string) {
    const next = selected.includes(val)
      ? selected.filter((v) => v !== val)
      : [...selected, val]
    onChange(next.join(','))
  }

  return (
    <div className="grid gap-3 rounded-lg border p-4 bg-muted/10 sm:grid-cols-2 lg:grid-cols-3">
      {options.map((opt) => (
        <div key={opt.value} className="flex items-center gap-2">
          <Checkbox
            id={`opt-${opt.value}`}
            checked={selected.includes(opt.value)}
            onCheckedChange={() => toggle(opt.value)}
            disabled={disabled}
          />
          <Label
            htmlFor={`opt-${opt.value}`}
            className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70 cursor-pointer"
          >
            {opt.label}
          </Label>
        </div>
      ))}
      {options.length === 0 && (
        <p className="col-span-full text-xs text-muted-foreground italic">No options available.</p>
      )}
    </div>
  )
}
