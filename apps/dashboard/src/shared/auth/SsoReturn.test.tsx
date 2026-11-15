import { screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderPage } from '@/test/page-harness'
import { AdminSsoCompleteDocument } from './Login.gql'
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
    at('?code=c1&state=s1', [{ request: { query: AdminSsoCompleteDocument, variables: { state: 's1', code: 'c1', binding: 'b1' } }, result: { data: { adminSsoComplete: payload } } }])
    expect(await screen.findByText('orders')).toBeDefined()
    expect(setAuth).toHaveBeenCalledWith(expect.objectContaining({ accessToken: 'a' }))
  })

  it('refuses a sign-in another tab or browser started', async () => {
    keepPendingSso({ state: 'mine', binding: 'b1', redirectTo: '/' })
    at('?code=c1&state=theirs')
    expect(await screen.findByText(/wasn't started in this browser tab/)).toBeDefined()
    await waitFor(() => expect(setAuth).not.toHaveBeenCalled())
  })

  it('shows the provider’s own refusal', async () => {
    keepPendingSso({ state: 's1', binding: 'b1', redirectTo: '/' })
    at('?error=access_denied&error_description=You+cancelled')
    expect(await screen.findByText('You cancelled')).toBeDefined()
  })
})
