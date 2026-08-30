import { afterEach, describe, expect, it } from 'vitest'
import type { AllowedChannelScopes } from '@/shared/auth/usePermissions'
import {
  DEFAULT_SCOPE,
  STORAGE_KEY,
  type ChannelScopeOption,
  filterChannelsByAllowedScopes,
  localesForAllowedChannel,
  readStoredScope,
  toScopeInput,
} from './config-scope'

const channel = (over: Partial<ChannelScopeOption> = {}): ChannelScopeOption => ({
  id: 'c1',
  name: 'US',
  defaultLocale: 'en',
  locales: [{ code: 'en' }, { code: 'es' }],
  isActive: true,
  ...over,
})

const all: AllowedChannelScopes = { allChannels: true, channelScopes: [] }
const only = (...scopes: { channelId: string; locales?: string[] | null }[]): AllowedChannelScopes => ({
  allChannels: false,
  channelScopes: scopes,
})

describe('filterChannelsByAllowedScopes', () => {
  it('drops inactive channels even from an admin who may see them all', () => {
    const channels = [channel(), channel({ id: 'c2', isActive: false })]
    expect(filterChannelsByAllowedScopes(channels, all).map((c) => c.id)).toEqual(['c1'])
  })

  it('keeps only the channels actually granted', () => {
    const channels = [channel(), channel({ id: 'c2' }), channel({ id: 'c3' })]
    expect(filterChannelsByAllowedScopes(channels, only({ channelId: 'c2' })).map((c) => c.id)).toEqual(['c2'])
  })

  it('shows nothing to an admin with no grants at all', () => {
    expect(filterChannelsByAllowedScopes([channel()], only())).toEqual([])
  })

  it('will not surface an inactive channel just because it was granted', () => {
    const channels = [channel({ id: 'c2', isActive: false })]
    expect(filterChannelsByAllowedScopes(channels, only({ channelId: 'c2' }))).toEqual([])
  })
})

describe('localesForAllowedChannel', () => {
  it('has nothing to offer without a channel', () => {
    expect(localesForAllowedChannel(null, all)).toEqual([])
  })

  it('offers the channel’s own locales to an unrestricted admin', () => {
    expect(localesForAllowedChannel(channel(), all)).toEqual(['en', 'es'])
  })

  it('falls back to the default locale for a channel that lists none', () => {
    expect(localesForAllowedChannel(channel({ locales: [] }), all)).toEqual(['en'])
  })

  it('narrows to the locales the grant names', () => {
    expect(localesForAllowedChannel(channel(), only({ channelId: 'c1', locales: ['es'] }))).toEqual(['es'])
  })

  // a grant with no locales means "any locale here", not "no locales"
  it('reads an empty locale list on a grant as the whole channel', () => {
    expect(localesForAllowedChannel(channel(), only({ channelId: 'c1', locales: [] }))).toEqual(['en', 'es'])
    expect(localesForAllowedChannel(channel(), only({ channelId: 'c1' }))).toEqual(['en', 'es'])
  })

  it('never invents a locale the channel does not have', () => {
    expect(localesForAllowedChannel(channel(), only({ channelId: 'c1', locales: ['de'] }))).toEqual([])
  })
})

describe('toScopeInput', () => {
  it('sends nothing at all for global, which is what the resolver reads as platform default', () => {
    expect(toScopeInput({ mode: 'GLOBAL', channelId: 'c1', locale: 'en' })).toBeUndefined()
  })

  it('names the channel, and the locale only when one is picked', () => {
    expect(toScopeInput({ mode: 'CHANNEL', channelId: 'c1', locale: 'en' })).toEqual({
      kind: 'CHANNEL', channelId: 'c1', locale: 'en',
    })
    // an empty locale must be absent, not sent as '' - the backend falls back on absence
    expect(toScopeInput({ mode: 'CHANNEL', channelId: 'c1', locale: '' })).toEqual({
      kind: 'CHANNEL', channelId: 'c1',
    })
  })
})

describe('readStoredScope', () => {
  afterEach(() => localStorage.clear())

  it('starts global when this device has never picked a scope', () => {
    expect(readStoredScope()).toEqual(DEFAULT_SCOPE)
  })

  it('reads back what it stored', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ mode: 'CHANNEL', channelId: 'c1', locale: 'es' }))
    expect(readStoredScope()).toEqual({ mode: 'CHANNEL', channelId: 'c1', locale: 'es' })
  })

  // storage is user-editable and outlives builds, so anything off falls back rather than throwing
  it('falls back on a mode it does not recognise', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ mode: 'SOMETHING', channelId: 'c1' }))
    expect(readStoredScope()).toEqual(DEFAULT_SCOPE)
  })

  it('falls back on unparseable storage', () => {
    localStorage.setItem(STORAGE_KEY, '{{ not json')
    expect(readStoredScope()).toEqual(DEFAULT_SCOPE)
  })

  it('repairs fields of the wrong type rather than passing them through', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ mode: 'CHANNEL', channelId: 42, locale: null }))
    expect(readStoredScope()).toEqual({ mode: 'CHANNEL', channelId: '', locale: '' })
  })
})
