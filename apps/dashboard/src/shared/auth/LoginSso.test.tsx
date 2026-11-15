import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderPage } from '@/test/page-harness'
import { AdminSsoOptionDocument, AdminSsoStartDocument } from './Login.gql'
import { Login } from './Login'
import { takePendingSso } from './sso'

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
    expect(takePendingSso()).toEqual({ state: 's1', binding: 'b1', redirectTo: '/' })
  })
})
