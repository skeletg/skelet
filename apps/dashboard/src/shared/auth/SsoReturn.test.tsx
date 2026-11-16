import { fireEvent, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderPage } from '@/test/page-harness'
import { AdminSsoCompleteDocument, AdminSsoLinkCompleteDocument, AdminSsoTotpDocument } from './Login.gql'
import { SsoReturn } from './SsoReturn'
import { keepPendingSso } from './sso'

const setAuth = vi.fn()
vi.mock('@/shared/auth/auth', async (original) => ({ ...(await original<object>()), useAuth: () => ({ auth: null, setAuth }) }))

const payload = {
  __typename: 'AdminAuthPayload' as const,
  accessToken: 'a',
  refreshToken: 'r',
  firstLogin: false,
  admin: { __typename: 'AdminUser' as const, id: 'u1', email: 'ops@acme.example', firstName: 'Ops', lastName: '', isSuperuser: false, roles: [] },
}

function at(query: string, mocks = [] as Parameters<typeof renderPage>[0]['mocks']) {
  renderPage({ path: `/login/sso${query}`, routes: { '/login/sso': <SsoReturn />, '/orders': <p>orders</p> }, mocks })
}

afterEach(() => {
  setAuth.mockReset()
  sessionStorage.clear()
})

describe('SsoReturn', () => {
  it('completes the sign-in this tab started and lands where it was headed', async () => {
    keepPendingSso({ state: 's1', binding: 'b1', redirectTo: '/orders' })
    at('?code=c1&state=s1', [{ request: { query: AdminSsoCompleteDocument, variables: { state: 's1', code: 'c1', binding: 'b1' } }, result: { data: { adminSsoComplete: { __typename: 'AdminSsoResult', session: payload, totpTicket: null } } } }])
    expect(await screen.findByText('orders')).toBeDefined()
    expect(setAuth).toHaveBeenCalledWith(expect.objectContaining({ accessToken: 'a' }))
  })

  it('asks for the admin’s own code when the provider didn’t prove a second factor', async () => {
    keepPendingSso({ state: 's1', binding: 'b1', redirectTo: '/orders' })
    at('?code=c1&state=s1', [
      { request: { query: AdminSsoCompleteDocument, variables: { state: 's1', code: 'c1', binding: 'b1' } }, result: { data: { adminSsoComplete: { __typename: 'AdminSsoResult', session: null, totpTicket: 't1' } } } },
      { request: { query: AdminSsoTotpDocument, variables: { ticket: 't1', code: '123456' } }, result: { data: { adminSsoTotp: payload } } },
    ])
    fireEvent.change(await screen.findByLabelText('Authentication code'), { target: { value: '123456' } })
    expect(setAuth).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(await screen.findByText('orders')).toBeDefined()
    expect(setAuth).toHaveBeenCalledWith(expect.objectContaining({ accessToken: 'a' }))
  })

  it('refuses a sign-in another tab or browser started, without asking the backend', async () => {
    keepPendingSso({ state: 'mine', binding: 'b1', redirectTo: '/' })
    const asked = vi.fn(() => ({ data: { adminSsoComplete: { __typename: 'AdminSsoResult', session: payload, totpTicket: null } } }))
    at('?code=c1&state=theirs', [{ request: { query: AdminSsoCompleteDocument, variables: { state: 'theirs', code: 'c1', binding: 'b1' } }, result: asked }])
    expect(await screen.findByText(/wasn't started in this browser tab/)).toBeDefined()
    expect(asked).not.toHaveBeenCalled()
    expect(setAuth).not.toHaveBeenCalled()
  })

  it('connects the signed-in admin’s provider account and goes back to where they started it', async () => {
    keepPendingSso({ state: 's1', binding: 'b1', redirectTo: '/orders', mode: 'link' })
    const linked = { __typename: 'AdminSsoLink' as const, id: 'l1', issuer: 'https://idp.example', email: 'ops@acme.example', createdAt: '2026-11-16T10:00:00Z', lastUsedAt: '2026-11-16T10:00:00Z' }
    at('?code=c1&state=s1', [{ request: { query: AdminSsoLinkCompleteDocument, variables: { state: 's1', code: 'c1', binding: 'b1' } }, result: { data: { adminSsoLinkComplete: linked } } }])
    expect(await screen.findByText('orders')).toBeDefined()
    expect(setAuth).not.toHaveBeenCalled()
  })

  it('says the provider turned it down in its own words, never the address bar’s', async () => {
    keepPendingSso({ state: 's1', binding: 'b1', redirectTo: '/' })
    at('?state=s1&error=access_denied&error_description=Call+this+number')
    expect(await screen.findByText(/You cancelled the sign-in/)).toBeDefined()
    expect(screen.queryByText(/Call this number/)).toBeNull()
  })

  it('treats a refusal for a sign-in it never started as foreign', async () => {
    at('?state=x&error=server_error')
    expect(await screen.findByText(/wasn't started in this browser tab/)).toBeDefined()
  })
})
