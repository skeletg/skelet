import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { DataOption } from './jobs'

// Rendered entirely from the registry descriptor, so a resource declaring a new option gets a working
// control here without a line of frontend change - which is the whole point of the backend being a registry
export function OptionsForm({
  options,
  values,
  onChange,
  direction,
}: {
  options: readonly DataOption[]
  values: Record<string, string>
  onChange: (name: string, value: string) => void
  direction: 'import' | 'export'
}) {
  const applicable = options.filter((o) => !o.appliesTo || o.appliesTo === direction)
  if (applicable.length === 0) return null

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {applicable.map((opt) => {
        const value = values[opt.name] ?? opt.default ?? ''
        // Already resolved server-side, including the entity pickers a Source provider enumerates
        const choices = opt.choices
        const id = `data-opt-${opt.name}`

        if (opt.type === 'bool') {
          return (
            <div key={opt.name} className="flex items-start gap-3 rounded-md border p-3">
              <Switch
                id={id}
                checked={value === 'true'}
                onCheckedChange={(on) => onChange(opt.name, on ? 'true' : 'false')}
              />
              <div className="grid gap-0.5">
                <Label htmlFor={id} className="cursor-pointer">
                  {opt.label || opt.name}
                </Label>
                {opt.description ? (
                  <p className="text-xs text-muted-foreground">{opt.description}</p>
                ) : null}
              </div>
            </div>
          )
        }

        return (
          <div key={opt.name} className="grid gap-1.5">
            <Label htmlFor={id}>
              {opt.label || opt.name}
              {opt.required ? <span className="ml-1 text-destructive">*</span> : null}
            </Label>
            {opt.type === 'select' || choices.length > 0 ? (
              <Select value={value} onValueChange={(v) => onChange(opt.name, v)}>
                <SelectTrigger id={id}>
                  <SelectValue placeholder={choices.length ? 'Choose one' : 'Nothing to choose from'} />
                </SelectTrigger>
                <SelectContent>
                  {choices.map((c) => (
                    <SelectItem key={c.value} value={c.value}>
                      {c.label || c.value}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                id={id}
                type={opt.type === 'date' ? 'date' : opt.type === 'int' ? 'number' : 'text'}
                value={value}
                placeholder={opt.default}
                onChange={(e) => onChange(opt.name, e.target.value)}
              />
            )}
            {opt.description ? (
              <p className="text-xs text-muted-foreground">{opt.description}</p>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

