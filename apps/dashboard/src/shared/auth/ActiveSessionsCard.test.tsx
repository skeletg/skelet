import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { meMock, renderDialog } from '@/test/page-harness'
import { ActiveSessionsCard } from './ActiveSessionsCard'
import { AuthContext, type AuthState } from './auth'
import { LogoutOtherDevicesDocument } from './Me.gql'

// What this pins: logging out other devices confirms through the shared ConfirmDialog - a refused logout keeps it
// open with the server's message, a successful one closes it - and without a refresh token it can't be started

const session = (id: string, isCurrent: boolean) => ({ __typename: 'AdminSession', id, ipAddress: '10.0.0.1', userAgent: 'Firefox', createdAt: '2026-10-01T00:00:00Z', lastActiveAt: '2026-10-01T00:00:00Z', isCurrent })
const base = meMock()
const me = { ...base, result: { data: { me: { ...(base.result as { data: { me: object } }).data.me, sessions: [session('s1', true), session('s2', false)] } } } }
const auth = { accessToken: 'at', refreshToken: 'rt', admin: {} } as AuthState
const logout = { query: LogoutOtherDevicesDocument, variables: { token: 'rt' } }

function renderCard(mocks: MockLink.MockedResponse[], state: AuthState | null = auth) {
  renderDialog(
    <AuthContext.Provider value={{ auth: state, setAuth: () => {} }}>
      <ActiveSessionsCard />
    </AuthContext.Provider>,
    [me, ...mocks],
  )
}

async function confirmLogout(mock: MockLink.MockedResponse) {
  renderCard([mock])
  const trigger = await screen.findByRole('button', { name: 'Log out other devices' })
  await waitFor(() => expect((trigger as HTMLButtonElement).disabled).toBe(false))
  fireEvent.click(trigger)
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Log out other devices' }))
  return dialog
}

describe('ActiveSessionsCard log out other devices', () => {
  it('keeps the dialog open and shows a refused logout', async () => {
    const dialog = await confirmLogout({ request: logout, result: { errors: [{ message: 'token expired' }] } })
    expect(await within(dialog).findByText('token expired')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes once the logout goes through', async () => {
    await confirmLogout({ request: logout, result: { data: { logoutOtherDevices: true } } })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })

  it('cannot start without a refresh token', async () => {
    renderCard([], null)
    await screen.findByText('Current session')
    expect((screen.getByRole('button', { name: 'Log out other devices' }) as HTMLButtonElement).disabled).toBe(true)
  })
})
