import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { usePickedExtra } from './product-picker-context'

// The per-pick inputs behind product-picker-columns - kept apart so that file exports builders only

export function QuantityInput({ id, label }: { id: string; label: string }) {
  const { value, patch } = usePickedExtra<{ quantity: number }>(id)
  return (
    <Input
      type="number"
      min={1}
      className="h-7 w-20 font-mono text-xs"
      aria-label={label}
      value={value?.quantity ?? 1}
      disabled={!value}
      onChange={(e) => patch({ quantity: Math.max(1, Number(e.target.value) || 1) })}
    />
  )
}

export function DefaultToggle({ id }: { id: string }) {
  const { value, patch } = usePickedExtra<{ isDefault: boolean }>(id)
  return (
    <Checkbox
      checked={value?.isDefault ?? false}
      disabled={!value}
      onCheckedChange={(v) => patch({ isDefault: v === true })}
      aria-label="Default selection"
    />
  )
}

// A configurable either stays one selection whose size the customer picks, or is split into its variants here - only
// configurable rows have the choice
export function EachVariantToggle({ id, kind }: { id: string; kind: string }) {
  const { value, patch } = usePickedExtra<{ eachVariant: boolean }>(id)
  if (kind !== 'CONFIGURABLE') return null
  return (
    <Checkbox
      checked={value?.eachVariant ?? false}
      disabled={!value}
      onCheckedChange={(v) => patch({ eachVariant: v === true })}
      aria-label="Add each variant as its own selection"
    />
  )
}
