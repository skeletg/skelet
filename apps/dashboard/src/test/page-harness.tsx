import type { ReactNode } from 'react'
import { render } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { MockedProvider } from '@apollo/client/testing/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ConfigScopeContext, type ConfigScopeContextValue } from '@/lib/config-scope'
import { MeDocument } from '@/shared/auth/Me.gql'

// Shared setup for the form page tests - a real page under Apollo's MockedProvider, a router and
// the header config scope, so each test only states its fixtures and what it clicks

// Save -> mutate -> navigate is several async hops - past testing-library's 1s default when the
// whole suite is loading the machine
export const SLOW = { timeout: 5000 }

// A mock's `variables` matcher that records the mutation input it was called with
export function captureInput(onInput: (input: Record<string, unknown>) => void) {
  return (vars: { input?: Record<string, unknown> } & Record<string, unknown>) => {
    onInput(vars.input ?? vars)
    return true
  }
}

// The `me` query usePermissions reads - a superuser passes every gate, or name the grants
export function meMock(opts: { superuser?: boolean; permissions?: string[] } = {}): MockLink.MockedResponse {
  const { superuser = true, permissions = [] } = opts
  return {
    request: { query: MeDocument },
    maxUsageCount: 20,
    result: {
      data: {
        me: {
          __typename: 'AdminUser',
          id: 'admin-1',
          email: 'admin@example.com',
          firstName: 'Ada',
          lastName: 'Admin',
          isActive: true,
          isSuperuser: superuser,
          totpEnabled: false,
          lastLoginAt: null,
          roles: [
            {
              __typename: 'Role',
              id: 'role-1',
              slug: 'test',
              name: 'Test',
              scopes: permissions.map((slug, i) => ({
                __typename: 'RolePermissionScope',
                channelId: null,
                locale: null,
                permission: { __typename: 'Permission', id: `p${i}`, slug, name: slug },
              })),
            },
          ],
          permissions: [],
          sessions: [],
        },
      },
    },
  }
}

// GLOBAL scope with `en` as the default locale
export const globalScope: ConfigScopeContextValue = {
  scope: { mode: 'GLOBAL', channelId: '', locale: '' },
  setScope: () => true,
  scopeInput: undefined,
  channels: [],
  selectedChannel: null,
  availableLocales: ['en'],
  loadingChannels: false,
  isGlobal: true,
  isChannel: false,
  effectiveLocale: 'en',
  effectiveChannelId: undefined,
  globalDefaultLocale: 'en',
  restrictedToChannels: false,
}

export function renderPage({
  path,
  routes,
  mocks,
  scope = globalScope,
}: {
  path: string
  /** Route pattern -> element; add the page a save navigates to, so the test can wait for it */
  routes: Record<string, ReactNode>
  mocks: MockLink.MockedResponse[]
  scope?: ConfigScopeContextValue
}) {
  return render(
    <MockedProvider mocks={mocks}>
      <ConfigScopeContext.Provider value={scope}>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            {Object.entries(routes).map(([pattern, element]) => (
              <Route key={pattern} path={pattern} element={element} />
            ))}
          </Routes>
        </MemoryRouter>
      </ConfigScopeContext.Provider>
    </MockedProvider>,
  )
}

// A dialog (or any component that isn't a routed page) under the same providers
export function renderDialog(node: ReactNode, mocks: MockLink.MockedResponse[] = [], scope: ConfigScopeContextValue = globalScope) {
  return render(
    <MockedProvider mocks={mocks}>
      <ConfigScopeContext.Provider value={scope}>{node}</ConfigScopeContext.Provider>
    </MockedProvider>,
  )
}

// Radix's Switch measures itself inside a <form>, and jsdom has no ResizeObserver - call at the top
// of a test file that renders one
export function stubResizeObserver() {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver
}
