import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { centsToDollars, dollarsToCents, parseNumberText } from '@/shared/schemas/fields'

// An offer line's unit price, applied when the field is left - like the quantity beside it. A
// price the admin set holds through checkout; reset puts a catalog line back on its catalog price
export function LinePrice({
  unitPrice,
  override,
  canReset,
  onCommit,
  onReset,
}: {
  unitPrice: number
  override: boolean
  canReset: boolean
  onCommit: (cents: number) => void
  onReset: () => void
}) {
  const [value, setValue] = useState(centsToDollars(unitPrice))
  const commit = () => {
    const n = parseNumberText(value)
    const cents = dollarsToCents(value)
    if (n !== null && n >= 0 && cents !== unitPrice) onCommit(cents)
    else setValue(centsToDollars(unitPrice))
  }
  return (
    <div className="flex items-center gap-1">
      <Input
        type="number"
        min={0}
        step="0.01"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
        }}
        className="w-24 tabular-nums"
        aria-label="Unit price"
      />
      {override && canReset && (
        <Button type="button" variant="ghost" size="sm" className="h-7 px-2 text-xs" onClick={onReset}>
          Reset
        </Button>
      )}
    </div>
  )
}
