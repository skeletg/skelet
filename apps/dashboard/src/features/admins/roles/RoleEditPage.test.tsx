import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { PermissionGroupsDocument } from '@/features/admins/permissions/Permissions.gql'
import { RoleEditPage } from './RoleEditPage'
import { CreateRoleDocument, RoleDocument, UpdateRoleDocument } from './Roles.gql'

// What these pin: a role with no grants is refused before anything is sent, a create lands on the
// new role's edit page, and an edit reviews each grant added or removed before writing the list

const permission = (slug: string) => ({ __typename: 'Permission' as const, id: `perm-${slug}`, slug, name: slug })

const shared: MockLink.MockedResponse[] = [
  {
    request: { query: PermissionGroupsDocument },
    result: {
      data: {
        permissionGroups: [
          { __typename: 'PermissionGroup', id: 'g-orders', slug: 'orders', name: 'Orders', permissions: [permission('orders.read'), permission('orders.write')] },
        ],
      },
    },
    maxUsageCount: 5,
  },
  { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 5 },
]

const role = {
  __typename: 'Role' as const,
  id: 'r1',
  slug: 'packer',
  name: 'Packer',
  description: '',
  memberCount: 0,
  createdAt: '2026-01-01T00:00:00Z',
  scopes: [{ __typename: 'RolePermissionScope' as const, channelId: null, locale: null, permission: permission('orders.read') }],
}

afterEach(() => localStorage.clear())

describe('RoleEditPage', () => {
  it('refuses a role without grants, then creates one and opens its edit page', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/admins/roles/new',
      routes: { '/admins/roles/new': <RoleEditPage />, '/admins/roles/:id/edit': <p>role edit page</p> },
      mocks: [
        ...shared,
        {
          request: { query: CreateRoleDocument, variables: captureInput(onCreate) },
          result: { data: { createRole: { __typename: 'Role', id: 'r9', slug: 'packer', name: 'Packer', scopes: [] } } },
        },
      ],
    })

    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'packer' } })
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Packer' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create role' }))
    expect(await screen.findByText(/A role with zero grants does nothing/)).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()

    fireEvent.click(await screen.findByRole('switch', { name: 'Grant orders.read' }))
    fireEvent.click(screen.getByRole('button', { name: 'Create role' }))
    expect(await screen.findByText('role edit page', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith({
      slug: 'packer',
      name: 'Packer',
      description: null,
      scopes: [{ permissionSlug: 'orders.read', channelId: null, locale: null }],
    })
  })

  it('reviews the grants added and removed, then writes the new list', async () => {
    const onUpdate = vi.fn()
    renderPage({
      path: '/admins/roles/r1/edit',
      routes: { '/admins/roles/:id/edit': <RoleEditPage /> },
      mocks: [
        ...shared,
        { request: { query: RoleDocument, variables: { id: 'r1' } }, result: { data: { role } }, maxUsageCount: 3 },
        {
          request: { query: UpdateRoleDocument, variables: captureInput(onUpdate) },
          result: { data: { updateRole: { __typename: 'Role', id: 'r1', slug: 'packer', name: 'Packer', scopes: [] } } },
        },
      ],
    })

    fireEvent.click(await screen.findByRole('switch', { name: 'Grant orders.read' }))
    fireEvent.click(screen.getByRole('switch', { name: 'Grant orders.write' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('orders.read · any channel · any locale')).toBeDefined()
    expect(within(dialog).getByText('orders.write · any channel · any locale')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))

    await vi.waitFor(
      () => expect(onUpdate).toHaveBeenCalledWith({ name: 'Packer', description: null, scopes: [{ permissionSlug: 'orders.write', channelId: null, locale: null }] }),
      SLOW,
    )
  })
})
