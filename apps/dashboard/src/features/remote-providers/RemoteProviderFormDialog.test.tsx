import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { RemoteProviderFormDialog } from './RemoteProviderFormDialog'
import { UpdateRemoteProviderDocument } from './RemoteProviders.gql'
import { toRemoteProviderUpdateInput, toRemoteProviderValues, type ExistingProvider } from './schemas/remoteProvider'

const provider: ExistingProvider = {
  id: 'rp1',
  kind: 'TAX_CALCULATOR' as ExistingProvider['kind'],
  slug: 'tax',
  name: 'Tax service',
  targetUrl: 'https://tax.example.com',
  config: { region: 'eu' },
  timeoutMs: 5000,
  cacheTtlS: 0,
  failurePolicy: 'FAIL_OPEN',
  channel: null,
}

describe('remote provider schema', () => {
  it('sends only what moved, and nothing when nothing did', () => {
    const values = toRemoteProviderValues(provider)
    expect(toRemoteProviderUpdateInput(values, provider)).toBeNull()
    expect(toRemoteProviderUpdateInput({ ...values, timeoutMs: '8000' }, provider)).toEqual({ id: 'rp1', timeoutMs: 8000 })
  })
})

describe('RemoteProviderFormDialog', () => {
  it('opens from its trigger, rejects broken JSON, then saves the change', async () => {
    const onInput = vi.fn()
    const onUpdated = vi.fn()
    renderDialog(<RemoteProviderFormDialog trigger={<button type="button">Edit</button>} existing={provider} onUpdated={onUpdated} />, [
      { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 3 },
      {
        request: {
          query: UpdateRemoteProviderDocument,
          variables: (vars: { input: unknown }) => {
            onInput(vars.input)
            return true
          },
        },
        result: { data: { updateRemoteProvider: { __typename: 'RemoteProvider', id: 'rp1' } } },
      },
    ])
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    fireEvent.change(await screen.findByLabelText('Config (JSON, optional)'), { target: { value: '{"region": ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(await screen.findByText('Config must be valid JSON')).toBeDefined()
    expect(onInput).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Config (JSON, optional)'), { target: { value: '{"region": "us"}' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await vi.waitFor(() => expect(onUpdated).toHaveBeenCalled(), SLOW)
    expect(onInput).toHaveBeenCalledWith({ id: 'rp1', config: { region: 'us' } })
  })
})
