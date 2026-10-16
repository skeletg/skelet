import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { RemoteProviderFormDialog } from './RemoteProviderFormDialog'
import { UpdateRemoteProviderDocument } from './RemoteProviders.gql'
import { toRemoteProviderCreateInput, toRemoteProviderUpdateInput, toRemoteProviderValues, type ExistingProvider } from './schemas/remoteProvider'

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

// A remote shipping carrier's weekend rule is its own switch, kept out of the JSON text and merged back into config
const carrier: ExistingProvider = {
  ...provider,
  id: 'rp2',
  kind: 'SHIPPING_CARRIER',
  slug: 'acme',
  name: 'Acme 3PL',
  config: { deliversOnWeekends: true, apiKey: 'k' },
}

describe('remote carrier weekend delivery', () => {
  it('lifts the weekend rule out of the JSON into its switch', () => {
    const values = toRemoteProviderValues(carrier)
    expect(values.deliversOnWeekends).toBe(true)
    expect(JSON.parse(values.config)).toEqual({ apiKey: 'k' })
    expect(toRemoteProviderValues(provider).deliversOnWeekends).toBe(false)
  })

  it('treats an untouched carrier as unchanged, whatever order its config keys came in', () => {
    expect(toRemoteProviderUpdateInput(toRemoteProviderValues(carrier), carrier)).toBeNull()
  })

  it('merges the switch back into config, and drops it when switched off', () => {
    const values = toRemoteProviderValues(carrier)
    expect(toRemoteProviderUpdateInput({ ...values, deliversOnWeekends: false }, carrier)).toEqual({ id: 'rp2', config: { apiKey: 'k' } })
    const plain = { ...carrier, config: null }
    expect(toRemoteProviderUpdateInput({ ...toRemoteProviderValues(plain), deliversOnWeekends: true }, plain)).toEqual({
      id: 'rp2',
      config: { deliversOnWeekends: true },
    })
  })

  it('creates a weekend carrier with the rule in its config', () => {
    const values = { ...toRemoteProviderValues(), slug: 'sat', name: 'Saturday Co', targetUrl: 'https://sat.example.com', deliversOnWeekends: true }
    expect(toRemoteProviderCreateInput(values).config).toEqual({ deliversOnWeekends: true })
  })

  it('leaves the config of any other role alone', () => {
    const values = { ...toRemoteProviderValues(provider), deliversOnWeekends: true }
    expect(toRemoteProviderUpdateInput(values, provider)).toBeNull()
  })

  it('shows the switch only for a shipping carrier, and saves it', async () => {
    const onInput = vi.fn()
    renderDialog(<RemoteProviderFormDialog trigger={<button type="button">Edit</button>} existing={{ ...carrier, config: { apiKey: 'k' } }} />, [
      { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 3 },
      {
        request: {
          query: UpdateRemoteProviderDocument,
          variables: (vars: { input: unknown }) => {
            onInput(vars.input)
            return true
          },
        },
        result: { data: { updateRemoteProvider: { __typename: 'RemoteProvider', id: 'rp2' } } },
      },
    ])
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    fireEvent.click(await screen.findByRole('checkbox', { name: /Delivers on weekends/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await vi.waitFor(() => expect(onInput).toHaveBeenCalledWith({ id: 'rp2', config: { apiKey: 'k', deliversOnWeekends: true } }), SLOW)
  })

  it('has no weekend switch for other roles', async () => {
    renderDialog(<RemoteProviderFormDialog trigger={<button type="button">Edit</button>} existing={provider} />, [
      { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 3 },
    ])
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    await screen.findByLabelText('Config (JSON, optional)')
    expect(screen.queryByRole('checkbox', { name: /Delivers on weekends/ })).toBeNull()
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
