import { useContext, useEffect } from 'react'
import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AllowedChannelScopes } from '@/shared/auth/usePermissions'
import { ConfigScopeContext, DEFAULT_SCOPE, STORAGE_KEY, type ConfigScopeContextValue } from './config-scope'
import { ConfigScopeProvider } from './config-scope-ui'

// The provider decides which channel an admin is looking at, so the behaviour worth pinning is what
// happens when the stored answer stops being one they are allowed to give - a revoked role, or the
// dev switcher swapping identities. Leaving them on it would show an empty header over another
// store's data
const { state } = vi.hoisted(() => ({
  state: {
    channels: [] as unknown[],
    allowed: { allChannels: true, channelScopes: [] } as AllowedChannelScopes,
    confirm: true,
    confirmCalls: 0,
    refetches: 0,
  },
}))

vi.mock('@apollo/client/react', () => ({
  useQuery: () => ({ data: { channels: state.channels }, loading: false }),
}))
vi.mock('@/shared/auth/usePermissions', () => ({
  usePermissions: () => ({ loading: false, allowedChannelScopes: () => state.allowed }),
}))
vi.mock('@/lib/apollo', () => ({
  apolloClient: {
    refetchQueries: () => {
      state.refetches += 1
      return Promise.resolve([])
    },
  },
}))
vi.mock('@/lib/locale-store', () => ({ setEffectiveLocale: () => {} }))
vi.mock('@/lib/channel-store', () => ({ setEffectiveChannelId: () => {} }))
vi.mock('@/lib/unsaved-changes', () => ({
  confirmScopeSwitch: () => {
    state.confirmCalls += 1
    return state.confirm
  },
  useUnsavedChangesGuard: () => {},
}))

const channel = (id: string, over: Partial<{ locales: { code: string }[]; defaultLocale: string }> = {}) => ({
  id,
  name: id.toUpperCase(),
  defaultLocale: 'en',
  locales: [{ code: 'en' }, { code: 'es' }],
  isActive: true,
  ...over,
})

let api: ConfigScopeContextValue

function Probe() {
  const ctx = useContext(ConfigScopeContext)!
  useEffect(() => {
    api = ctx
  })
  return null
}

function mount(permissionSlug?: string) {
  return render(
    <ConfigScopeProvider permissionSlug={permissionSlug}>
      <Probe />
    </ConfigScopeProvider>,
  )
}

const store = (scope: unknown) => localStorage.setItem(STORAGE_KEY, JSON.stringify(scope))

beforeEach(() => {
  localStorage.clear()
  state.channels = [channel('c1'), channel('c2')]
  state.allowed = { allChannels: true, channelScopes: [] }
  state.confirm = true
  state.confirmCalls = 0
  state.refetches = 0
})
afterEach(() => localStorage.clear())

describe('an admin who may see every channel', () => {
  it('stays global, which is where an unrestricted admin belongs', () => {
    mount()
    expect(api.scope).toEqual(DEFAULT_SCOPE)
    expect(api.isGlobal).toBe(true)
    expect(api.restrictedToChannels).toBe(false)
  })

  it('sends no channel id or scope input in global', () => {
    mount()
    expect(api.effectiveChannelId).toBeUndefined()
    expect(api.scopeInput).toBeUndefined()
  })

  it('honours a stored channel scope', () => {
    store({ mode: 'CHANNEL', channelId: 'c2', locale: 'es' })
    mount()
    expect(api.scope.channelId).toBe('c2')
    expect(api.isChannel).toBe(true)
    expect(api.effectiveChannelId).toBe('c2')
    expect(api.scopeInput).toEqual({ kind: 'CHANNEL', channelId: 'c2', locale: 'es' })
  })
})

describe('an admin restricted to certain channels', () => {
  beforeEach(() => {
    state.allowed = { allChannels: false, channelScopes: [{ channelId: 'c2', locales: null }] }
  })

  // a global selection from a previous identity leaves the header saying "Global default" over an
  // empty channel field, so it snaps to somewhere they can actually be
  it('snaps a stored global scope into their first permitted channel', () => {
    mount('settings.manage')
    expect(api.scope).toEqual({ mode: 'CHANNEL', channelId: 'c2', locale: '' })
    expect(api.restrictedToChannels).toBe(true)
  })

  it('snaps off a channel they can no longer reach', () => {
    store({ mode: 'CHANNEL', channelId: 'c1', locale: '' })
    mount('settings.manage')
    expect(api.scope.channelId).toBe('c2')
  })

  it('offers only the channels they hold', () => {
    mount('settings.manage')
    expect(api.channels.map((c) => c.id)).toEqual(['c2'])
  })

  it('clears a locale the grant does not cover', () => {
    state.allowed = { allChannels: false, channelScopes: [{ channelId: 'c2', locales: ['es'] }] }
    store({ mode: 'CHANNEL', channelId: 'c2', locale: 'en' })
    mount('settings.manage')
    expect(api.scope.locale).toBe('')
    expect(api.availableLocales).toEqual(['es'])
  })

  it('keeps a locale the grant does cover', () => {
    state.allowed = { allChannels: false, channelScopes: [{ channelId: 'c2', locales: ['es'] }] }
    store({ mode: 'CHANNEL', channelId: 'c2', locale: 'es' })
    mount('settings.manage')
    expect(api.scope.locale).toBe('es')
  })

  // correcting on an empty list would snap to nothing and then snap again once it arrives
  it('waits for the channel list before correcting anything', () => {
    state.channels = []
    store({ mode: 'GLOBAL', channelId: '', locale: '' })
    mount('settings.manage')
    expect(api.scope.mode).toBe('GLOBAL')
  })
})

describe('effectiveLocale', () => {
  it('prefers the explicit pick', () => {
    store({ mode: 'CHANNEL', channelId: 'c1', locale: 'es' })
    mount()
    expect(api.effectiveLocale).toBe('es')
  })

  it('falls back to the channel default when nothing is picked', () => {
    store({ mode: 'CHANNEL', channelId: 'c1', locale: '' })
    mount()
    expect(api.effectiveLocale).toBe('en')
  })

  it('is empty in global, so the backend picks', () => {
    mount()
    expect(api.effectiveLocale).toBe('')
  })
})

describe('switching scope by hand', () => {
  it('asks before discarding staged edits, and writes when allowed', () => {
    mount()
    let ok = false
    act(() => {
      ok = api.setScope({ mode: 'CHANNEL', channelId: 'c1', locale: '' })
    })
    expect(ok).toBe(true)
    expect(state.confirmCalls).toBe(1)
    expect(api.scope.channelId).toBe('c1')
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!).channelId).toBe('c1')
  })

  it('changes nothing when the prompt is declined', () => {
    state.confirm = false
    mount()
    let ok = true
    act(() => {
      ok = api.setScope({ mode: 'CHANNEL', channelId: 'c1', locale: '' })
    })
    expect(ok).toBe(false)
    expect(api.scope).toEqual(DEFAULT_SCOPE)
  })

  // the provider's own auto-correction goes through setScope, and prompting there would ask about
  // a change the admin never made
  it('skips the prompt for a scope that is already current', () => {
    mount()
    act(() => {
      api.setScope({ ...DEFAULT_SCOPE })
    })
    expect(state.confirmCalls).toBe(0)
  })
})
