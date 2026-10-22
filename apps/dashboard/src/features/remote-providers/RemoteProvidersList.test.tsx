import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { RemoteProvidersList } from './RemoteProvidersList'
import { RemoteProvidersDocument, RotateRemoteProviderSecretDocument } from './RemoteProviders.gql'

// What this pins: rotating a secret breaks every caller still signing with the old one, so it asks first and only
// rotates on confirm

const provider = {
  __typename: 'RemoteProvider' as const,
  id: 'rp1',
  kind: 'pricing',
  slug: 'erp',
  name: 'ERP pricing',
  targetUrl: 'https://erp.example.com',
  config: {},
  timeoutMs: 1000,
  cacheTtlS: 0,
  failurePolicy: 'fail_open',
  isActive: true,
  circuit: { __typename: 'CircuitState', state: 'closed', consecutiveFailures: 0 },
  channel: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

describe('RemoteProvidersList rotate secret', () => {
  it('rotates only after the admin confirms', async () => {
    const rotated = vi.fn()
    renderPage({
      path: '/remote-providers',
      routes: { '/remote-providers': <RemoteProvidersList /> },
      adminId: 'admin-1',
      mocks: [
        meMock(),
        { request: { query: RemoteProvidersDocument }, result: { data: { remoteProviders: [provider] } }, maxUsageCount: 3 },
        {
          request: {
            query: RotateRemoteProviderSecretDocument,
            variables: (vars: Record<string, unknown>) => {
              rotated(vars)
              return true
            },
          },
          result: {
            data: { rotateRemoteProviderSecret: { __typename: 'RemoteProviderSecretResult', rawSecret: 'whsec_new', provider } },
          },
        },
      ],
    })
    const trigger = await screen.findByRole('button', { name: 'Actions for ERP pricing' }, SLOW)
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rotate secret' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(rotated).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Rotate secret' }))
    await vi.waitFor(() => expect(rotated).toHaveBeenCalledWith({ id: 'rp1' }), SLOW)
  })
})
