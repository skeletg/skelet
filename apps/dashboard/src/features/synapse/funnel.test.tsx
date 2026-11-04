import { fireEvent, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { SynapseFunnelDocument, SynapseOverviewDocument } from './Synapse.gql'
import { SynapseFunnel } from './SynapseFunnel'

const channelScope = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'c1', locale: '' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'c1',
  selectedChannel: { id: 'c1', name: 'Main', slug: 'main' },
} as unknown as ConfigScopeContextValue

describe('SynapseFunnel', () => {
  it('sends a step filter and a breakdown, and shows the breakdown', async () => {
    const asked: Record<string, unknown>[] = []
    const funnel = (breakdown: unknown[]) => ({
      request: {
        query: SynapseFunnelDocument,
        variables: (v: Record<string, unknown>) => {
          asked.push(v)
          return true
        },
      },
      maxUsageCount: 10,
      result: {
        data: {
          synapseFunnel: {
            __typename: 'SynapseFunnel',
            steps: ['page_view', 'view_item', 'cart_add', 'begin_checkout', 'order_placed'].map((eventName, i) => ({ __typename: 'SynapseFunnelStep', eventName, count: 10 - i })),
            breakdown,
          },
        },
      },
    })
    renderPage({
      path: '/x',
      routes: { '/x': <SynapseFunnel /> },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [
        meMock(),
        { request: { query: SynapseOverviewDocument, variables: () => true }, maxUsageCount: 5, error: new Error('no overview') },
        funnel([{ __typename: 'SynapseFunnelBreakdown', value: 'mobile', counts: [6, 5, 4, 3, 2] }]),
      ],
    })

    fireEvent.click(await screen.findByRole('button', { name: 'Filter step 1' }, SLOW))
    fireEvent.change(screen.getByRole('textbox', { name: 'Property for step 1' }), { target: { value: 'device' } })
    fireEvent.change(screen.getByRole('textbox', { name: 'Value for step 1' }), { target: { value: 'mobile' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add filter' }))
    await waitFor(() => expect(asked.some((v) => JSON.stringify(v.filters) === JSON.stringify([{ step: 0, key: 'device', value: 'mobile' }]))).toBe(true), SLOW)

    const by = screen.getByRole('textbox', { name: /Break down by/ })
    fireEvent.change(by, { target: { value: 'device' } })
    fireEvent.blur(by)
    await waitFor(() => expect(asked.some((v) => v.breakdown === 'device')).toBe(true), SLOW)
    expect(await screen.findByRole('cell', { name: 'mobile' }, SLOW)).toBeTruthy()
    expect(screen.getByText('33%')).toBeTruthy() // 2 of the 6 who started converted
  })
})
