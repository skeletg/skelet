import { fromMinor, toMinor } from '@/lib/format'
import type { ChannelPrice, ChannelPriceEdits } from './productOptionsStaging'

export type SurchargeChannel = { id: string; name: string; currencyCode: string }

// Amounts are typed the way a person reads them ("4.50"), each in its own channel's currency
export function rawChannelPrices(
  prices: readonly ChannelPrice[] | undefined,
  channels: readonly SurchargeChannel[],
): Record<string, string> {
  const currency = new Map(channels.map((c) => [c.id, c.currencyCode]))
  return Object.fromEntries(
    (prices ?? []).map((p) => {
      const code = currency.get(p.channelId)
      return [p.channelId, code ? fromMinor(p.priceValue, code) : String(p.priceValue)]
    }),
  )
}

export function invalidChannelPrices(channels: readonly SurchargeChannel[], raw: Record<string, string>): SurchargeChannel[] {
  return channels.filter((ch) => (raw[ch.id] ?? '').trim() !== '' && toMinor(raw[ch.id], ch.currencyCode) === null)
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
    const next = text === '' ? null : toMinor(text, ch.currencyCode)
    if (text !== '' && next === null) continue
    if (next !== before) edits[ch.id] = next
  }
  return Object.keys(edits).length > 0 ? edits : undefined
}
