import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toMinor } from '@/lib/format'
import type { SurchargeChannel } from './channelSurcharges'

export function ChannelSurchargeInputs({
  idPrefix,
  channels,
  baseCurrency,
  baseAmount,
  values,
  disabled = false,
  onChange,
}: {
  idPrefix: string
  channels: readonly SurchargeChannel[]
  baseCurrency: string
  baseAmount: number
  values: Record<string, string>
  disabled?: boolean
  onChange: (channelId: string, raw: string) => void
}) {
  if (channels.length === 0) return null
  return (
    <div className="grid max-w-xl gap-2">
      <p className="text-xs text-muted-foreground">
        Leave a store empty to charge the amount above where it sells in {baseCurrency}. A store selling in another
        currency needs its own amount, or customers there can't pick this.
      </p>
      <div className="divide-y rounded-md border">
        {channels.map((ch) => {
          const sameCurrency = ch.currencyCode.toUpperCase() === baseCurrency.toUpperCase()
          const raw = values[ch.id] ?? ''
          const id = `${idPrefix}-${ch.id}`
          const invalid = raw.trim() !== '' && toMinor(raw, ch.currencyCode) === null
          return (
            <div key={ch.id} className="grid gap-1 px-3 py-2">
              <div className="flex items-center justify-between gap-3">
                <Label htmlFor={id} className="text-sm font-normal">
                  {ch.name} <span className="text-muted-foreground">{ch.currencyCode}</span>
                </Label>
                <Input
                  id={id}
                  inputMode="decimal"
                  className="w-28 text-right tabular-nums"
                  placeholder={sameCurrency ? 'Same' : 'Not set'}
                  value={raw}
                  disabled={disabled}
                  aria-invalid={invalid || undefined}
                  onChange={(e) => onChange(ch.id, e.target.value)}
                />
              </div>
              {invalid ? (
                <p className="text-xs text-destructive">Not a valid {ch.currencyCode} amount.</p>
              ) : !sameCurrency && baseAmount !== 0 && raw.trim() === '' ? (
                <p className="text-xs text-warning">Not sold on {ch.name} until set.</p>
              ) : null}
            </div>
          )
        })}
      </div>
    </div>
  )
}
