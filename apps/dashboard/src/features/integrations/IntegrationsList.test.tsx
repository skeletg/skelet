import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { IntegrationsList } from './IntegrationsList'
import { IntegrationsDocument, RegenerateIntegrationTokenDocument } from './Integrations.gql'

// What this pins: the row menu's "Regenerate token" kills a live credential, so it asks first and sends nothing on cancel

const integration = {
  __typename: 'Integration' as const,
  id: 'i1',
  name: 'ERP sync',
  description: '',
  isActive: true,
  createdAt: '2026-01-01T00:00:00Z',
  expiresAt: null,
  lastUsedAt: null,
  channelId: null,
  permissions: [],
}

describe('IntegrationsList regenerate token', () => {
  it('confirms before regenerating and sends nothing on cancel', async () => {
    const regenerated = vi.fn()
    renderPage({
      path: '/integrations',
      routes: { '/integrations': <IntegrationsList /> },
      adminId: 'admin-1',
      mocks: [
        meMock(),
        { request: { query: IntegrationsDocument }, result: { data: { integrations: [integration] } }, maxUsageCount: 3 },
        {
          request: {
            query: RegenerateIntegrationTokenDocument,
            variables: (vars: Record<string, unknown>) => {
              regenerated(vars)
              return true
            },
          },
          result: { data: { regenerateIntegrationToken: { __typename: 'IntegrationTokenResult', rawToken: 'sk_new', integration } } },
        },
      ],
    })
    const trigger = await screen.findByRole('button', { name: 'Actions for ERP sync' }, SLOW)
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Regenerate token' }))

    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await vi.waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), SLOW)
    expect(regenerated).not.toHaveBeenCalled()
  })
})
