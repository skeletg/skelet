import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { centsToDollars, dollarsToCents, parseNumberText } from '@/shared/schemas/fields'

export const MAX_PRICE_REASON = 200 // cart.MaxPriceReason

// An offer line's unit price, applied when the field is left - like the quantity beside it. A
// price the admin set holds through checkout; reset puts a catalog line back on its catalog price.
// A set price can carry a note saying why, which the order line keeps - reason is null on a line that takes none
export function LinePrice({
  unitPrice,
  reason,
  override,
  canReset,
  onCommit,
  onReset,
}: {
  unitPrice: number
  reason: string | null
  override: boolean
  canReset: boolean
  onCommit: (cents: number, reason: string) => void
  onReset: () => void
}) {
  const [value, setValue] = useState(centsToDollars(unitPrice))
  const [why, setWhy] = useState(reason ?? '')
  const commit = () => {
    const n = parseNumberText(value)
    const cents = dollarsToCents(value)
    if (n !== null && n >= 0 && cents !== unitPrice) onCommit(cents, why.trim())
    else setValue(centsToDollars(unitPrice))
  }
  const blurOnEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') e.currentTarget.blur()
  }
  return (
    <div className="grid gap-1">
      <div className="flex items-center gap-1">
        <Input
          type="number"
          min={0}
          step="0.01"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onBlur={commit}
          onKeyDown={blurOnEnter}
          className="w-24 tabular-nums"
          aria-label="Unit price"
        />
        {override && canReset && (
          <Button type="button" variant="ghost" size="sm" className="h-7 px-2 text-xs" onClick={onReset}>
            Reset
          </Button>
        )}
      </div>
      {override && reason !== null && (
        <Input
          value={why}
          maxLength={MAX_PRICE_REASON}
          onChange={(e) => setWhy(e.target.value)}
          onBlur={() => {
            if (why.trim() !== reason) onCommit(unitPrice, why.trim())
          }}
          onKeyDown={blurOnEnter}
          placeholder="Why this price"
          className="h-7 text-xs"
          aria-label="Reason for the price"
        />
      )}
    </div>
  )
}
