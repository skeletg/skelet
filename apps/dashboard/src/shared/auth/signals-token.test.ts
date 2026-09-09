import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { STORAGE_KEY } from './auth'

// This exchange sits on the hot path of every Prism screen - mint too often and each panel costs a round trip to core, cache too long and a logged-out session keeps a working analytics credential

function storeSession(accessToken: string) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ accessToken }))
}

function mintResponse(token: string, expiresInMs: number) {
  return {
    ok: true,
    json: async () => ({
      data: { signalsToken: { token, expiresAt: new Date(Date.now() + expiresInMs).toISOString() } },
    }),
  }
}

// resetModules gives signals-token its own copy of ./auth, so both must come from the same graph or the listener it subscribed to is not the one the test notifies
async function freshModule() {
  vi.resetModules()
  const [tokens, auth] = await Promise.all([import('./signals-token'), import('./auth')])
  return { ...tokens, ...auth }
}

describe('signals token exchange', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    localStorage.clear()
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('mints once and reuses the token while it is fresh', async () => {
    storeSession('admin-token')
    fetchMock.mockResolvedValue(mintResponse('signals-token', 15 * 60_000))
    const { getSignalsToken } = await freshModule()

    expect(await getSignalsToken()).toBe('signals-token')
    expect(await getSignalsToken()).toBe('signals-token')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('asks core once when several panels open at the same time', async () => {
    storeSession('admin-token')
    fetchMock.mockResolvedValue(mintResponse('signals-token', 15 * 60_000))
    const { getSignalsToken } = await freshModule()

    const all = await Promise.all([getSignalsToken(), getSignalsToken(), getSignalsToken()])

    expect(all).toEqual(['signals-token', 'signals-token', 'signals-token'])
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('re-mints before expiry rather than sending a token that dies mid-request', async () => {
    storeSession('admin-token')
    // Inside the renewal margin, so the cached one must not be reused
    fetchMock.mockResolvedValue(mintResponse('signals-token', 30_000))
    const { getSignalsToken } = await freshModule()

    await getSignalsToken()
    await getSignalsToken()

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('drops the cached token when the admin session changes', async () => {
    storeSession('admin-token')
    fetchMock.mockResolvedValue(mintResponse('signals-token', 15 * 60_000))
    const { getSignalsToken, setAuthStoredState } = await freshModule()

    await getSignalsToken()
    expect(fetchMock).toHaveBeenCalledTimes(1)

    setAuthStoredState(null) // logout
    storeSession('another-admin')
    await getSignalsToken()

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('asks for nothing when there is no session to exchange', async () => {
    const { getSignalsToken } = await freshModule()

    expect(await getSignalsToken()).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('returns null rather than a broken token when core refuses', async () => {
    storeSession('admin-token')
    fetchMock.mockResolvedValue({ ok: false, json: async () => ({}) })
    const { getSignalsToken } = await freshModule()

    expect(await getSignalsToken()).toBeNull()
  })
})
