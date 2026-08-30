import { useMemo } from 'react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

type TimeRange = {
  from: string // HH:MM
  to: string   // HH:MM
}

type WorkingHours = Record<string, {
  enabled: boolean
  ranges: TimeRange[]
}>

const DAYS = [
  { key: 'mon', label: 'Monday' },
  { key: 'tue', label: 'Tuesday' },
  { key: 'wed', label: 'Wednesday' },
  { key: 'thu', label: 'Thursday' },
  { key: 'fri', label: 'Friday' },
  { key: 'sat', label: 'Saturday' },
  { key: 'sun', label: 'Sunday' },
]

export function WorkingHoursEditor({
  value,
  onChange,
  disabled,
}: {
  value: string
  onChange: (v: string) => void
  disabled?: boolean
}) {
  const data = useMemo(() => {
    try {
      return JSON.parse(value) as WorkingHours
    } catch {
      return {} as WorkingHours
    }
  }, [value])

  function update(next: WorkingHours) {
    onChange(JSON.stringify(next))
  }

  function toggleDay(day: string, enabled: boolean) {
    const next = { ...data }
    if (!next[day]) {
      next[day] = { enabled, ranges: [{ from: '09:00', to: '17:00' }] }
    } else {
      next[day] = { ...next[day], enabled }
    }
    update(next)
  }

  function addRange(day: string) {
    const next = { ...data }
    if (!next[day]) {
      next[day] = { enabled: true, ranges: [{ from: '09:00', to: '17:00' }] }
    } else {
      next[day].ranges = [...next[day].ranges, { from: '09:00', to: '17:00' }]
    }
    update(next)
  }

  function removeRange(day: string, idx: number) {
    const next = { ...data }
    next[day].ranges = next[day].ranges.filter((_, i) => i !== idx)
    if (next[day].ranges.length === 0) {
      next[day].enabled = false
    }
    update(next)
  }

  function updateRange(day: string, idx: number, key: keyof TimeRange, val: string) {
    const next = { ...data }
    next[day].ranges[idx] = { ...next[day].ranges[idx], [key]: val }
    update(next)
  }

  return (
    <div className="grid gap-4 rounded-xl border bg-muted/20 p-4">
      {DAYS.map(({ key, label }) => {
        const dayConfig = data[key] || { enabled: false, ranges: [] }
        return (
          <div key={key} className="grid gap-3 sm:grid-cols-[120px_1fr] items-start">
            <div className="flex items-center gap-2 pt-1">
              <Checkbox
                id={`day-${key}`}
                checked={dayConfig.enabled}
                onCheckedChange={(v) => toggleDay(key, !!v)}
                disabled={disabled}
              />
              <Label htmlFor={`day-${key}`} className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
                {label}
              </Label>
            </div>

            <div className="grid gap-2">
              {dayConfig.enabled ? (
                <>
                  {dayConfig.ranges.map((range, idx) => (
                    <div key={idx} className="flex items-center gap-2">
                      <Input
                        type="time"
                        value={range.from}
                        onChange={(e) => updateRange(key, idx, 'from', e.target.value)}
                        disabled={disabled}
                        className="h-8 w-32"
                      />
                      <span className="text-muted-foreground">to</span>
                      <Input
                        type="time"
                        value={range.to}
                        onChange={(e) => updateRange(key, idx, 'to', e.target.value)}
                        disabled={disabled}
                        className="h-8 w-32"
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-muted-foreground hover:text-destructive"
                        onClick={() => removeRange(key, idx)}
                        disabled={disabled}
                      >
                        <Trash2Icon className="size-4" />
                      </Button>
                    </div>
                  ))}
                  <Button
                    variant="outline"
                    size="sm"
                    className="w-fit h-7 px-2 text-[10px] uppercase tracking-wider"
                    onClick={() => addRange(key)}
                    disabled={disabled}
                  >
                    <PlusIcon className="size-3 mr-1" />
                    Add Range
                  </Button>
                </>
              ) : (
                <span className="text-xs text-muted-foreground italic pt-1">Closed</span>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}
