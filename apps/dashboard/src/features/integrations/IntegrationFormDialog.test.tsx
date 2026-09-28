import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { PermissionGroupsDocument } from '@/features/admins/permissions/Permissions.gql'
import { IntegrationFormDialog } from './IntegrationFormDialog'
import { CreateIntegrationDocument, IntegrationsDocument } from './Integrations.gql'
import { toIntegrationUpdateInput, toIntegrationValues } from './schemas/integration'

const pickers = [
  { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 3 },
  { request: { query: PermissionGroupsDocument }, result: { data: { permissionGroups: [] } }, maxUsageCount: 3 },
  { request: { query: IntegrationsDocument }, result: { data: { integrations: [] } }, maxUsageCount: 3 },
]

describe('integration schema', () => {
  it('clears what an edit switched off', () => {
    const values = toIntegrationValues({ id: 'i1', name: 'Feed', description: '', expiresAt: '2030-01-01T00:00:00Z', channelId: 'c1', permissions: [] })
    expect(values.hasExpiry).toBe(true)
    expect(toIntegrationUpdateInput({ ...values, hasExpiry: false, channelId: '' }, 'i1')).toMatchObject({
      clearExpiresAt: true,
      clearChannel: true,
    })
  })
})

describe('IntegrationFormDialog', () => {
  it('asks for a date once expiry is ticked, then hands the new token over', async () => {
    const onCreateInput = vi.fn()
    const onCreated = vi.fn()
    renderDialog(<IntegrationFormDialog open onOpenChange={() => {}} onCreated={onCreated} />, [
      ...pickers,
      {
        request: { query: CreateIntegrationDocument, variables: captureInput(onCreateInput) },
        result: {
          data: {
            createIntegration: {
              __typename: 'CreateIntegrationPayload',
              rawToken: 'sk_live_once',
              integration: { __typename: 'Integration', id: 'i1', name: 'Feed' },
            },
          },
        },
      },
    ])
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Feed' } })
    fireEvent.click(screen.getByLabelText('Set expiry date'))
    fireEvent.click(screen.getByRole('button', { name: 'Create integration' }))
    expect(await screen.findByText('Pick when the token expires')).toBeDefined()

    fireEvent.change(screen.getByLabelText('Expires at'), { target: { value: '2030-06-01T12:00' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create integration' }))
    await vi.waitFor(() => expect(onCreated).toHaveBeenCalledWith({ token: 'sk_live_once', name: 'Feed' }), SLOW)
    expect(onCreateInput.mock.calls[0][0]).toMatchObject({ name: 'Feed', channelId: null })
    expect(onCreateInput.mock.calls[0][0].expiresAt).toBe(new Date('2030-06-01T12:00').toISOString())
  })
})
