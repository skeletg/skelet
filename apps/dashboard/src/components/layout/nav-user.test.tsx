import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SidebarProvider } from '@/components/ui/sidebar'
import { AuthContext } from '@/shared/auth/auth'
import { renderPage } from '@/test/page-harness'
import { NavUser } from './nav-user'

// What this pins: Change password still opens now that its dialog (and the form stack behind it) loads on first use

window.matchMedia ??= ((query: string) =>
  ({ matches: false, media: query, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, onchange: null, dispatchEvent: () => false }) as MediaQueryList)

const auth = { accessToken: 't', admin: { id: 'a1', email: 'ada@example.com', firstName: 'Ada', lastName: 'Admin' } } as never

describe('NavUser change password', () => {
  it('opens the lazily loaded dialog from the account menu', async () => {
    renderPage({
      path: '/',
      mocks: [],
      routes: {
        '/': (
          <AuthContext.Provider value={{ auth, setAuth: () => {} }}>
            <SidebarProvider>
              <NavUser />
            </SidebarProvider>
          </AuthContext.Provider>
        ),
      },
    })
    expect(screen.queryByRole('dialog')).toBeNull()
    fireEvent.pointerDown(screen.getByText('Ada Admin'), { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /Change password/ }))
    expect(await screen.findByRole('dialog', {}, { timeout: 5000 })).toBeTruthy()
    expect(screen.getAllByText('Change password').length).toBeGreaterThan(0)
  })
})
