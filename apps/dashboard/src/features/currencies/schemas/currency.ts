import { z } from 'zod'
import { currencyCode, numberText, wholeNumber } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import {
  cleanMetadata,
  metadataDiff,
  metadataSchema,
  toMetadataValues,
  type MetadataEntry,
} from '@/shared/schemas/metadata'

// Currency form - THE field list (docs/conventions.md, "Record forms")

export type ExistingCurrency = {
  code: string
  name: string
  symbol: string
  decimals: number
  symbolPosition: string
  isActive: boolean
  sortOrder: number
  metadata: MetadataEntry[]
  exchangeRate?: number | null
  rateUpdatedAt?: string | null
}

export const currencySchema = z
  .object({
    code: currencyCode().describe('Upper-case ISO 4217 code, such as USD - set on create, never changed'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the currency'),
    symbol: z.string().describe('Symbol prices are shown with, such as $ - empty string for none'),
    decimals: wholeNumber().min(0, 'Between 0 and 4').max(4, 'Between 0 and 4').describe('Decimal places amounts are shown with, 0 to 4'),
    // The service's own vocabulary - uppercase BEFORE/AFTER fails its check in internal/currency/service.go
    symbolPosition: z.enum(['prefix', 'suffix']).describe('Whether the symbol goes before or after the amount'),
    isActive: z.boolean().describe('Whether the currency is available for use'),
    sortOrder: wholeNumber().describe('Order in currency lists - lower first'),
    exchangeRate: numberText({ optional: true, above: 0 }, 'Must be a positive number').describe(
      'Manual exchange-rate override, a positive number written as text, edit only - empty string leaves the rate to the currency-rates job',
    ),
    metadata: metadataSchema,
  })
  .describe('A currency')

export type CurrencyValues = z.infer<typeof currencySchema>

export const CURRENCY_LABELS: Record<keyof CurrencyValues, string> = {
  code: 'Code',
  name: 'Name',
  symbol: 'Symbol',
  decimals: 'Decimals',
  symbolPosition: 'Symbol position',
  isActive: 'Active',
  sortOrder: 'Sort order',
  exchangeRate: 'Exchange rate',
  metadata: 'Metadata',
}

export function toCurrencyValues(currency?: ExistingCurrency): CurrencyValues {
  return {
    code: currency?.code ?? '',
    name: currency?.name ?? '',
    symbol: currency?.symbol ?? '',
    decimals: currency?.decimals ?? 2,
    symbolPosition: currency?.symbolPosition === 'suffix' ? 'suffix' : 'prefix',
    isActive: currency?.isActive ?? true,
    sortOrder: currency?.sortOrder ?? 0,
    exchangeRate: currency?.exchangeRate != null ? String(currency.exchangeRate) : '',
    metadata: toMetadataValues(currency?.metadata),
  }
}

export function toCurrencyInput(v: CurrencyValues) {
  return {
    code: v.code,
    name: v.name.trim(),
    symbol: v.symbol,
    decimals: v.decimals,
    symbolPosition: v.symbolPosition,
    isActive: v.isActive,
    sortOrder: v.sortOrder,
    metadata: cleanMetadata(v.metadata, { trimValues: true }),
  }
}

// Only an operator-changed rate is sent - the job owns it otherwise, and echoing the fetched value
// back would pin it as a manual override
export function exchangeRateOverride(v: CurrencyValues, stored?: number | null): { exchangeRate?: number } {
  const rate = Number(v.exchangeRate)
  return v.exchangeRate.trim() !== '' && Number.isFinite(rate) && rate > 0 && rate !== stored
    ? { exchangeRate: rate }
    : {}
}

// Code is immutable after create, so it never appears
const DIFFED = ['name', 'symbol', 'decimals', 'symbolPosition', 'isActive', 'sortOrder', 'exchangeRate'] as const

export function currencyDiff(original: CurrencyValues, draft: CurrencyValues): SectionDiffEntry[] {
  return [
    ...fieldDiff(
      original,
      draft,
      DIFFED.map((k) => ({ label: CURRENCY_LABELS[k], value: (o: CurrencyValues) => o[k] })),
    ),
    ...metadataDiff(original.metadata, draft.metadata),
  ]
}
