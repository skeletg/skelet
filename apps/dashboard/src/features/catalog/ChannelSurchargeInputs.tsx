import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { ChannelPrice, ChannelPriceEdits } from './productOptionsStaging'

export type SurchargeChannel = { id: string; name: string; currencyCode: string }

export function rawChannelPrices(prices: readonly ChannelPrice[] | undefined): Record<string, string> {
  return Object.fromEntries((prices ?? []).map((p) => [p.channelId, String(p.priceValue)]))
}

// Only what moved - an emptied input clears the channel's amount, an unparseable one is left alone
export function channelEditsFrom(
  channels: readonly SurchargeChannel[],
  raw: Record<string, string>,
  prev: readonly ChannelPrice[] | undefined,
): ChannelPriceEdits | undefined {
  const edits: ChannelPriceEdits = {}
  for (const ch of channels) {
    const before = prev?.find((p) => p.channelId === ch.id)?.priceValue ?? null
    const text = (raw[ch.id] ?? '').trim()
    const next = text === '' ? null : Number(text)
    if (next !== null && !Number.isInteger(next)) continue
    if (next !== before) edits[ch.id] = next
  }
  return Object.keys(edits).length > 0 ? edits : undefined
}

export function ChannelSurchargeInputs({
  idPrefix,
  channels,
  baseCurrency,
  baseAmount,
  values,
  onChange,
}: {
  idPrefix: string
  channels: readonly SurchargeChannel[]
  baseCurrency: string
  baseAmount: number
  values: Record<string, string>
  onChange: (channelId: string, raw: string) => void
}) {
  if (channels.length === 0) return null
  return (
    <div className="grid gap-2">
      <Label>Per-channel amount</Label>
      <p className="text-xs text-muted-foreground">
        Empty uses the amount above on channels selling in {baseCurrency}. Anywhere else a non-zero amount
        can't be charged, so the choice can't be picked until it has its own, in that channel's currency.
      </p>
      <div className="divide-y rounded-md border">
        {channels.map((ch) => {
          const sameCurrency = ch.currencyCode.toUpperCase() === baseCurrency.toUpperCase()
          const raw = values[ch.id] ?? ''
          const id = `${idPrefix}-${ch.id}`
          return (
            <div key={ch.id} className="grid gap-1 px-3 py-2">
              <div className="flex items-center justify-between gap-3">
                <Label htmlFor={id} className="text-sm font-normal">
                  {ch.name} <span className="text-muted-foreground">{ch.currencyCode}</span>
                </Label>
                <Input
                  id={id}
                  type="number"
                  step={1}
                  className="w-28"
                  placeholder={sameCurrency ? 'base' : 'not set'}
                  value={raw}
                  onChange={(e) => onChange(ch.id, e.target.value)}
                />
              </div>
              {!sameCurrency && baseAmount !== 0 && raw.trim() === '' && (
                <p className="text-xs text-warning">Not sold on {ch.name} until set.</p>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
