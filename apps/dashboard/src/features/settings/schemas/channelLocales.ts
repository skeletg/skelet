import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, changed, removed } from '@/shared/saving/diff'

// A channel's languages on the Settings page - THE field list (docs/conventions.md, "Record forms"):
// the supported set and the default, which must be one of them

export const channelLocalesSchema = z
  .object({
    codes: z.array(z.string()),
    defaultLocale: z.string(),
  })
  .refine((v) => v.defaultLocale === '' || v.codes.includes(v.defaultLocale), {
    message: 'The default locale must be one of the supported locales',
    path: ['defaultLocale'],
  })

export type ChannelLocalesValues = z.infer<typeof channelLocalesSchema>

export function toChannelLocalesValues(channel: { defaultLocale: string; locales: { code: string }[] }): ChannelLocalesValues {
  return { codes: channel.locales.map((l) => l.code), defaultLocale: channel.defaultLocale }
}

const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((c) => b.includes(c))

export type LocalesStep = { kind: 'codes'; codes: string[] } | { kind: 'default'; code: string }

// The writes, in an order the backend's "default must stay supported" guard accepts: the outgoing
// default stays in the set until the new default lands, then drops out
export function channelLocalesSteps(before: ChannelLocalesValues, after: ChannelLocalesValues): LocalesStep[] {
  const codesChanged = !sameSet(before.codes, after.codes)
  const defaultChanged = before.defaultLocale !== after.defaultLocale
  if (!codesChanged) return defaultChanged ? [{ kind: 'default', code: after.defaultLocale }] : []
  if (!defaultChanged) return [{ kind: 'codes', codes: after.codes }]
  const keepOld = !after.codes.includes(before.defaultLocale)
  return [
    { kind: 'codes', codes: keepOld ? [...after.codes, before.defaultLocale] : after.codes },
    { kind: 'default', code: after.defaultLocale },
    ...(keepOld ? [{ kind: 'codes' as const, codes: after.codes }] : []),
  ]
}

export function channelLocalesDiff(original: ChannelLocalesValues, draft: ChannelLocalesValues): SectionDiffEntry[] {
  const def = changed('Default locale', original.defaultLocale, draft.defaultLocale)
  return [
    ...[...draft.codes].sort().filter((c) => !original.codes.includes(c)).map((c) => added(`Locale · ${c}`)),
    ...[...original.codes].sort().filter((c) => !draft.codes.includes(c)).map((c) => removed(`Locale · ${c}`)),
    ...(def ? [def] : []),
  ]
}
