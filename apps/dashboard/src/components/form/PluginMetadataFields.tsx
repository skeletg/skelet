import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import type { DetailFieldEntry } from '@/plugins/field-registry'

type Row = { key: string; value: string }

const NONE = '__none'

export function PluginMetadataFields({ fields, rows, onChange }: { fields: DetailFieldEntry[]; rows: Row[]; onChange: (rows: Row[]) => void }) {
  const valueOf = (key: string) => rows.find((r) => r.key.trim() === key)?.value ?? ''
  const setValue = (key: string, value: string) => {
    const at = rows.findIndex((r) => r.key.trim() === key)
    if (value === '') onChange(rows.filter((_, i) => i !== at))
    else if (at >= 0) onChange(rows.map((r, i) => (i === at ? { key, value } : r)))
    else onChange([...rows, { key, value }])
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {fields.map((f) => {
        const id = `plugin-field-${f.id.replace(/[^a-z0-9_-]/gi, '-')}`
        const value = valueOf(f.key)
        const hint = f.hint && <p className="text-xs text-muted-foreground">{f.hint}</p>
        if (f.type === 'checkbox') {
          return (
            <div key={f.id} className="grid gap-1.5 sm:col-span-2">
              <div className="flex items-center gap-2">
                <Checkbox id={id} checked={value === 'true'} onCheckedChange={(v) => setValue(f.key, v ? 'true' : '')} />
                <Label htmlFor={id} className="cursor-pointer">{f.label}</Label>
              </div>
              {hint}
            </div>
          )
        }
        return (
          <div key={f.id} className={f.type === 'textarea' ? 'grid gap-1.5 sm:col-span-2' : 'grid gap-1.5'}>
            <Label htmlFor={id}>{f.label}</Label>
            {f.type === 'textarea' ? (
              <Textarea id={id} rows={3} value={value} onChange={(e) => setValue(f.key, e.target.value)} />
            ) : f.type === 'select' ? (
              <Select value={value || NONE} onValueChange={(v) => setValue(f.key, v === NONE ? '' : v)}>
                <SelectTrigger id={id} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>None</SelectItem>
                  {f.options?.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                id={id}
                type={f.type === 'number' ? 'number' : f.type === 'date' ? 'date' : 'text'}
                value={value}
                onChange={(e) => setValue(f.key, e.target.value)}
              />
            )}
            {hint}
          </div>
        )
      })}
    </div>
  )
}
