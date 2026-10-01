import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { BusinessIdFieldValues } from './schemas/country'

const EMPTY: BusinessIdFieldValues = { key: '', label: '', required: false, pattern: '', hint: '' }

// The company identifiers a business purchase from this country asks for at checkout, in the order shown - Serbia's
// PIB and Matični broj, a VAT number elsewhere. Staged like every other field; the page's Save applies them
export function BusinessIdFieldsEditor({
  value,
  onChange,
  errors,
}: {
  value: BusinessIdFieldValues[]
  onChange: (next: BusinessIdFieldValues[]) => void
  errors?: string[]
}) {
  const set = (i: number, patch: Partial<BusinessIdFieldValues>) =>
    onChange(value.map((f, j) => (j === i ? { ...f, ...patch } : f)))
  return (
    <div className="grid gap-3">
      {value.length === 0 ? (
        <p className="rounded-md border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">
          None - a business purchase from this country asks for one generic tax ID, as the checkout settings say.
        </p>
      ) : (
        <div className="divide-y rounded-md border">
          {value.map((f, i) => (
            <div key={i} className="grid gap-3 p-3">
              <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-[2fr_1fr_auto]">
                <div className="grid gap-1.5">
                  <Label htmlFor={`bid-${i}-label`}>Label</Label>
                  <Input
                    id={`bid-${i}-label`}
                    value={f.label}
                    placeholder="PIB"
                    onChange={(e) => set(i, { label: e.target.value })}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor={`bid-${i}-key`}>Key</Label>
                  <Input
                    id={`bid-${i}-key`}
                    value={f.key}
                    placeholder="from the label"
                    className="font-mono"
                    onChange={(e) => set(i, { key: e.target.value.toLowerCase() })}
                  />
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-8 sm:mt-6"
                  aria-label={`Remove ${f.label || `field ${i + 1}`}`}
                  onClick={() => onChange(value.filter((_, j) => j !== i))}
                >
                  <Trash2Icon className="size-4" />
                </Button>
              </div>
              <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label htmlFor={`bid-${i}-pattern`}>Format (regular expression)</Label>
                  <Input
                    id={`bid-${i}-pattern`}
                    value={f.pattern}
                    placeholder="\d{9}"
                    className="font-mono"
                    onChange={(e) => set(i, { pattern: e.target.value })}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor={`bid-${i}-hint`}>Hint for the shopper</Label>
                  <Input
                    id={`bid-${i}-hint`}
                    value={f.hint}
                    placeholder="9 digits"
                    onChange={(e) => set(i, { hint: e.target.value })}
                  />
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`bid-${i}-required`}
                  checked={f.required}
                  onCheckedChange={(v) => set(i, { required: !!v })}
                />
                <Label htmlFor={`bid-${i}-required`} className="cursor-pointer">
                  Required - a business purchase can't be placed without it
                </Label>
              </div>
            </div>
          ))}
        </div>
      )}
      {errors && errors.length > 0 && <p className="text-xs text-destructive">{errors.join(' ')}</p>}
      <Button type="button" variant="outline" size="sm" className="w-fit" onClick={() => onChange([...value, EMPTY])}>
        <PlusIcon className="size-4" />
        Add field
      </Button>
    </div>
  )
}
