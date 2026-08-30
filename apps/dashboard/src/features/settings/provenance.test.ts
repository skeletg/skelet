import { describe, expect, it } from 'vitest'
import type { ConfigFieldData } from '@/features/settings/ConfigField'
import { fieldIsInherited, fieldMatchesScope } from './provenance'

// The resolver always answers with the EFFECTIVE value, so a channel pane is mostly platform
// defaults wearing the channel's clothes - `scope` is the only thing that tells the two apart, and
// the sidebar's override count and every provenance rail are read off it

const field = (over: Partial<ConfigFieldData> = {}): ConfigFieldData => ({
  path: 'general/store/name',
  section: 'general',
  group: 'store',
  field: 'name',
  label: 'Store name',
  type: 'STRING',
  value: 'Acme',
  isSet: false,
  isDefault: false,
  public: true,
  scoping: 'CHANNEL',
  scope: { kind: 'DEFAULT' },
  ...over,
})

describe('fieldMatchesScope', () => {
  it('matches a platform default in global mode', () => {
    expect(fieldMatchesScope({ kind: 'DEFAULT' }, 'GLOBAL', undefined, '')).toBe(true)
  })

  it('does not count a channel override as global', () => {
    expect(fieldMatchesScope({ kind: 'CHANNEL', channelId: 'c1' }, 'GLOBAL', undefined, '')).toBe(false)
  })

  it('matches a channel override only for the channel being viewed', () => {
    const scope = { kind: 'CHANNEL', channelId: 'c1' }
    expect(fieldMatchesScope(scope, 'CHANNEL', 'c1', '')).toBe(true)
    expect(fieldMatchesScope(scope, 'CHANNEL', 'c2', '')).toBe(false)
  })

  it('pairs a locale override with the locale being viewed, and only that one', () => {
    const scope = { kind: 'CHANNEL', channelId: 'c1', locale: 'en' }
    expect(fieldMatchesScope(scope, 'CHANNEL', 'c1', 'en')).toBe(true)
    expect(fieldMatchesScope(scope, 'CHANNEL', 'c1', 'fr')).toBe(false)
  })

  it('treats no locale asked for as a demand for the unlocalised row', () => {
    // otherwise an en-only override would light up the rail while no locale is selected
    expect(fieldMatchesScope({ kind: 'CHANNEL', channelId: 'c1', locale: 'en' }, 'CHANNEL', 'c1', '')).toBe(false)
    expect(fieldMatchesScope({ kind: 'CHANNEL', channelId: 'c1' }, 'CHANNEL', 'c1', '')).toBe(true)
  })
})

describe('fieldIsInherited', () => {
  it('calls a registry default inherited whatever its scope says', () => {
    expect(fieldIsInherited(field({ isDefault: true, scope: { kind: 'CHANNEL', channelId: 'c1' } }), 'CHANNEL', 'c1', '')).toBe(true)
  })

  it('calls a value set right here its own', () => {
    expect(fieldIsInherited(field({ scope: { kind: 'CHANNEL', channelId: 'c1' } }), 'CHANNEL', 'c1', '')).toBe(false)
  })

  it('calls a global value showing through a channel pane inherited', () => {
    expect(fieldIsInherited(field({ scope: { kind: 'DEFAULT' } }), 'CHANNEL', 'c1', '')).toBe(true)
  })

  it('calls another channel’s value inherited rather than mine', () => {
    expect(fieldIsInherited(field({ scope: { kind: 'CHANNEL', channelId: 'other' } }), 'CHANNEL', 'c1', '')).toBe(true)
  })
})
