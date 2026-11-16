import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderPage } from '@/test/page-harness'
import { AdminSsoOptionDocument, AdminSsoStartDocument } from './Login.gql'
import { Login } from './Login'
import { keepPendingSso, providerUrl, takePendingSso } from './sso'

vi.mock('@/shared/auth/auth', async (original) => ({ ...(await original<object>()), useAuth: () => ({ auth: null, setAuth: vi.fn() }) }))

afterEach(() => vi.unstubAllGlobals())

describe('Login with a provider', () => {
  it('offers the provider and sends the browser there, keeping what only this tab may finish with', async () => {
    const assign = vi.fn()
    vi.stubGlobal('location', { ...window.location, origin: 'http://localhost:5173', assign })
    renderPage({
      path: '/login',
      routes: { '/login': <Login /> },
      mocks: [
        { request: { query: AdminSsoOptionDocument }, result: { data: { adminSsoOption: { __typename: 'SsoOption', label: 'Sign in with Acme' } } } },
        {
          request: { query: AdminSsoStartDocument, variables: { redirectUri: 'http://localhost:5173/login/sso' } },
          result: { data: { adminSsoStart: { __typename: 'SsoStart', authorizeUrl: 'https://idp.example/authorize?x', state: 's1', binding: 'b1' } } },
        },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Sign in with Acme' }))
    await waitFor(() => expect(assign).toHaveBeenCalledWith('https://idp.example/authorize?x'))
    expect(takePendingSso()).toEqual({ state: 's1', binding: 'b1', redirectTo: '/', mode: 'signin' })
  })

  it('never follows a provider address that would run in the dashboard', async () => {
    const assign = vi.fn()
    vi.stubGlobal('location', { ...window.location, origin: 'http://localhost:5173', assign })
    renderPage({
      path: '/login',
      routes: { '/login': <Login /> },
      mocks: [
        { request: { query: AdminSsoOptionDocument }, result: { data: { adminSsoOption: { __typename: 'SsoOption', label: 'Sign in with Acme' } } } },
        {
          request: { query: AdminSsoStartDocument, variables: { redirectUri: 'http://localhost:5173/login/sso' } },
          result: { data: { adminSsoStart: { __typename: 'SsoStart', authorizeUrl: 'javascript:alert(1)//', state: 's1', binding: 'b1' } } },
        },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Sign in with Acme' }))
    expect(await screen.findByText(/isn't a safe one to open/)).toBeDefined()
    expect(assign).not.toHaveBeenCalled()
    expect(takePendingSso()).toBeNull()
  })
})

describe('what the starting tab keeps', () => {
  it('only sends the browser back within the dashboard, and only to the provider over https', () => {
    for (const [kept, back] of [['/orders', '/orders'], ['//evil.example', '/'], ['https://evil.example', '/']]) {
      keepPendingSso({ state: 's', binding: 'b', redirectTo: kept })
      expect(takePendingSso()?.redirectTo).toBe(back)
    }
    expect(providerUrl('https://idp.example/authorize')).toBe('https://idp.example/authorize')
    expect(providerUrl('http://localhost:8180/auth')).toBe('http://localhost:8180/auth')
    for (const bad of ['javascript:alert(1)', 'http://idp.example/authorize', 'data:text/html,x', 'nonsense']) {
      expect(providerUrl(bad)).toBeNull()
    }
  })
})
