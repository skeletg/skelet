import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { AuthContext } from '@/shared/auth/auth'
import { RolesDocument } from '@/features/admins/roles/Roles.gql'
import { AdminDetail } from './AdminDetail'
import { AdminDocument, DeleteAdminDocument, UpdateAdminDocument } from './Admins.gql'

// What these pin: profile and role edits are reviewed together and written in one update, a blank
// name keeps the confirm dialog open, and the role list is only sent when it changed

const role = (slug: string, name: string) => ({
  __typename: 'Role' as const,
  id: `role-${slug}`,
  slug,
  name,
  description: '',
  memberCount: 1,
  scopes: [],
})

const admin = {
  __typename: 'AdminUser' as const,
  id: 'a1',
  email: 'jane@example.com',
  firstName: 'Jane',
  lastName: 'Doe',
  isActive: true,
  isSuperuser: false,
  createdAt: '2026-01-01T00:00:00Z',
  roles: [{ __typename: 'Role' as const, id: 'role-packer', slug: 'packer', name: 'Packer' }],
  permissions: [],
}

function renderAdmin(onUpdate = vi.fn(), extra: MockLink.MockedResponse[] = []) {
  renderPage({
    path: '/admins/a1',
    routes: {
      '/admins/:id': (
        <AuthContext.Provider value={{ auth: null, setAuth: () => {} }}>
          <AdminDetail />
        </AuthContext.Provider>
      ),
      '/admins': <p>Admins list</p>,
    },
    mocks: [
      ...extra,
      meMock(),
      { request: { query: AdminDocument, variables: { id: 'a1' } }, result: { data: { admin } }, maxUsageCount: 3 },
      { request: { query: RolesDocument }, result: { data: { roles: [role('packer', 'Packer'), role('support', 'Support')] } }, maxUsageCount: 3 },
      {
        request: { query: UpdateAdminDocument, variables: captureInput(onUpdate) },
        result: { data: { updateAdmin: { __typename: 'AdminUser', id: 'a1', roles: [] } } },
      },
    ],
  })
  return onUpdate
}

describe('AdminDetail', () => {
  it('reviews a name and a role change together, then writes them in one update', async () => {
    const onUpdate = renderAdmin()
    fireEvent.change(await screen.findByLabelText('Last name'), { target: { value: 'Smith' } })
    fireEvent.click(await screen.findByRole('checkbox', { name: /Support/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Role · support')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(
      () =>
        expect(onUpdate).toHaveBeenCalledWith({ firstName: 'Jane', lastName: 'Smith', isActive: true, roleSlugs: ['packer', 'support'] }),
      SLOW,
    )
  })

  it('keeps the dialog open on a blank name, and leaves the roles alone when they did not change', async () => {
    const onUpdate = renderAdmin()
    const first = await screen.findByLabelText('First name')
    // The page is read-only - no Save - until the signed-in admin's permissions have loaded
    await screen.findByRole('button', { name: 'Save changes' })
    fireEvent.change(first, { target: { value: ' ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/First name is required/)).toBeDefined()
    expect(onUpdate).not.toHaveBeenCalled()

    fireEvent.change(first, { target: { value: 'Janet' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ firstName: 'Janet', lastName: 'Doe', isActive: true }), SLOW)
  })
})

// The delete confirm runs through the shared ConfirmDialog: a refused delete keeps it open with the server's message,
// a successful one goes back to the list
describe('AdminDetail delete', () => {
  async function confirmDelete(mock: MockLink.MockedResponse) {
    renderAdmin(vi.fn(), [mock])
    fireEvent.click(await screen.findByRole('button', { name: 'Delete admin' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete admin' }))
    return dialog
  }

  it('keeps the dialog open and shows a refused delete', async () => {
    const dialog = await confirmDelete({ request: { query: DeleteAdminDocument, variables: { id: 'a1' } }, result: { errors: [{ message: 'last superuser' }] } })
    expect(await within(dialog).findByText('last superuser')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteAdminDocument, variables: { id: 'a1' } }, result: { data: { deleteAdmin: true } } })
    expect(await screen.findByText('Admins list', {}, SLOW)).toBeDefined()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
