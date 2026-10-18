import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { PermissionGroupsDocument } from '@/features/admins/permissions/Permissions.gql'
import { IntegrationDetail } from './IntegrationDetail'
import { IntegrationsDocument, SetIntegrationActiveDocument, UpdateIntegrationDocument } from './Integrations.gql'

// What these pin: the active switch and the profile are reviewed together, each write goes only when
// its part changed, and switching the expiry off clears it rather than leaving it out

const integration = {
  __typename: 'Integration' as const,
  id: 'i1',
  name: 'ERP sync',
  description: '',
  isActive: true,
  createdAt: '2026-01-01T00:00:00Z',
  expiresAt: '2027-01-01T10:00:00Z',
  channelId: null,
  permissions: [{ __typename: 'Permission' as const, id: 'p1', slug: 'orders.read', name: 'orders.read' }],
}

function renderIntegration(writes = { active: vi.fn(), update: vi.fn() }) {
  renderPage({
    path: '/integrations/i1',
    routes: { '/integrations/:id': <IntegrationDetail /> },
    mocks: [
      { request: { query: IntegrationsDocument }, result: { data: { integrations: [integration] } }, maxUsageCount: 3 },
      { request: { query: PermissionGroupsDocument }, result: { data: { permissionGroups: [] } }, maxUsageCount: 3 },
      {
        request: {
          query: SetIntegrationActiveDocument,
          variables: (vars: Record<string, unknown>) => {
            writes.active(vars)
            return true
          },
        },
        result: { data: { setIntegrationActive: { __typename: 'Integration', id: 'i1', isActive: false } } },
      },
      {
        request: { query: UpdateIntegrationDocument, variables: captureInput(writes.update) },
        result: { data: { updateIntegration: { __typename: 'Integration', id: 'i1', name: 'ERP' } } },
      },
    ],
  })
  return writes
}

describe('IntegrationDetail', () => {
  it('reviews status and profile together, then writes both', async () => {
    const writes = renderIntegration()
    fireEvent.click(await screen.findByRole('switch', { name: 'Toggle active' }))
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'ERP' } })
    fireEvent.click(screen.getByLabelText('Set expiry date'))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 3 changes/ }))
    await vi.waitFor(() => expect(writes.update).toHaveBeenCalled(), SLOW)
    expect(writes.active).toHaveBeenCalledWith({ id: 'i1', active: false })
    expect(writes.update).toHaveBeenCalledWith({
      id: 'i1',
      name: 'ERP',
      description: '',
      permissionSlugs: ['orders.read'],
      clearExpiresAt: true,
    })
  })

  it('sends only the active switch when nothing else changed', async () => {
    const writes = renderIntegration()
    fireEvent.click(await screen.findByRole('switch', { name: 'Toggle active' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(writes.active).toHaveBeenCalledWith({ id: 'i1', active: false }), SLOW)
    expect(writes.update).not.toHaveBeenCalled()
  })
})
