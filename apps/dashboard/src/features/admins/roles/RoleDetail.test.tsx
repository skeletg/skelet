import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { PermissionGroupsDocument } from '@/features/admins/permissions/Permissions.gql'
import { AdminsDocument } from '@/features/admins/Admins.gql'
import { RoleDetail } from './RoleDetail'
import { DeleteRoleDocument, RoleDocument } from './Roles.gql'

// What this pins: the role delete confirm runs through the shared ConfirmDialog - a refused delete keeps it open
// with the server's message, a successful one goes back to the role list

const role = { __typename: 'Role', id: 'r1', slug: 'packer', name: 'Packer', description: '', memberCount: 0, createdAt: '2026-01-01T00:00:00Z', scopes: [] }

async function confirmDelete(mock: MockLink.MockedResponse) {
  renderPage({
    path: '/admins/roles/r1',
    adminId: 'admin-1',
    routes: { '/admins/roles/:id': <RoleDetail />, '/admins/roles': <p>Roles list</p> },
    mocks: [
      meMock(),
      { request: { query: RoleDocument, variables: { id: 'r1' } }, result: { data: { role } }, maxUsageCount: 5 },
      { request: { query: AdminsDocument }, result: { data: { admins: [] } }, maxUsageCount: 5 },
      { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 5 },
      { request: { query: PermissionGroupsDocument }, result: { data: { permissionGroups: [] } }, maxUsageCount: 5 },
      mock,
    ],
  })
  // Delete sits behind admins.manage - wait for the grants before clicking it
  fireEvent.click(await screen.findByRole('button', { name: 'Delete' }, SLOW))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete role' }))
  return dialog
}

describe('RoleDetail delete', () => {
  it('keeps the dialog open and shows a refused delete', async () => {
    const dialog = await confirmDelete({ request: { query: DeleteRoleDocument, variables: { id: 'r1' } }, result: { errors: [{ message: 'role has members' }] } })
    expect(await within(dialog).findByText('role has members')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteRoleDocument, variables: { id: 'r1' } }, result: { data: { deleteRole: true } } })
    expect(await screen.findByText('Roles list', {}, SLOW)).toBeDefined()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
