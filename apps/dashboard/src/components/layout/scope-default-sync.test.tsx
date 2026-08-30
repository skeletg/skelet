import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ScopeDefaultSync } from './scope-default-sync'

const { state } = vi.hoisted(() => ({
  state: {
    pref: null as unknown,
    loaded: true,
    hadStoredScope: false,
    channels: [] as Array<{ id: string; name: string }>,
    applied: [] as unknown[],
  },
}))

vi.mock('@/lib/admin-prefs', () => ({
  usePreference: () => [state.pref, () => {}],
  usePrefsLoaded: () => state.loaded,
}))

vi.mock('@/lib/config-scope', () => ({
  get hadStoredScope() {
    return state.hadStoredScope
  },
  useConfigScope: () => ({
    channels: state.channels,
    setScope: (next: unknown) => state.applied.push(next),
  }),
}))

const CHANNEL = { mode: 'CHANNEL', channelId: 'ch-1', locale: 'en' }

afterEach(cleanup)
beforeEach(() => {
  state.pref = null
  state.loaded = true
  state.hadStoredScope = false
  state.channels = [{ id: 'ch-1', name: 'Main' }]
  state.applied = []
})

describe('ScopeDefaultSync', () => {
  it('seeds a device that has never picked a scope', () => {
    state.pref = CHANNEL
    render(<ScopeDefaultSync />)
    expect(state.applied).toEqual([CHANNEL])
  })

  // roaming the live scope would move another session's scope mid-task, so a device that already
  // chose keeps its choice
  it('leaves a device that already has a scope alone', () => {
    state.pref = CHANNEL
    state.hadStoredScope = true
    render(<ScopeDefaultSync />)
    expect(state.applied).toEqual([])
  })

  it('waits for the roaming copy rather than acting on the default', () => {
    state.pref = null
    state.loaded = false
    render(<ScopeDefaultSync />)
    expect(state.applied).toEqual([])
  })

  it('drops a default pointing at a channel this admin lost', () => {
    state.pref = { mode: 'CHANNEL', channelId: 'gone', locale: '' }
    render(<ScopeDefaultSync />)
    expect(state.applied).toEqual([])
  })

  it('applies a global default without waiting on channels', () => {
    state.pref = { mode: 'GLOBAL', channelId: '', locale: '' }
    state.channels = []
    render(<ScopeDefaultSync />)
    expect(state.applied).toEqual([{ mode: 'GLOBAL', channelId: '', locale: '' }])
  })
})
