import { describe, expect, it } from 'vitest'
import { channelLocalesSteps } from './channelLocales'

// What these pin: the writes never leave the channel with an unsupported default, whatever changed

describe('channelLocalesSteps', () => {
  const before = { codes: ['en', 'de'], defaultLocale: 'en' }

  it('writes nothing when nothing changed', () => {
    expect(channelLocalesSteps(before, { codes: ['de', 'en'], defaultLocale: 'en' })).toEqual([])
  })

  it('writes only the set, or only the default, when only one changed', () => {
    expect(channelLocalesSteps(before, { codes: ['en', 'de', 'fr'], defaultLocale: 'en' })).toEqual([
      { kind: 'codes', codes: ['en', 'de', 'fr'] },
    ])
    expect(channelLocalesSteps(before, { codes: ['en', 'de'], defaultLocale: 'de' })).toEqual([{ kind: 'default', code: 'de' }])
  })

  it('keeps the outgoing default supported until the new one lands', () => {
    expect(channelLocalesSteps(before, { codes: ['de', 'fr'], defaultLocale: 'fr' })).toEqual([
      { kind: 'codes', codes: ['de', 'fr', 'en'] },
      { kind: 'default', code: 'fr' },
      { kind: 'codes', codes: ['de', 'fr'] },
    ])
  })
})
