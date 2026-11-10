import { render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { AssistantReportDocument } from '@/features/dashboard/Assistant.gql'
import { AssistantWidget } from './assistant'

vi.mock('@/lib/config-scope', () => ({ useConfigScope: () => ({ effectiveChannelId: 'ch1' }) }))

const report = (over: { cost: number | null; orders: object[] }) => ({
  request: { query: AssistantReportDocument, variables: { channelId: 'ch1', days: 30 } },
  result: {
    data: {
      assistantReport: {
        __typename: 'AssistantReport',
        days: 30,
        turns: 40,
        failedTurns: 2,
        unansweredTurns: 6,
        cartTurns: 12,
        checkoutTurns: 5,
        usage: { __typename: 'AssistantUsage', inputTokens: 100_000, outputTokens: 50_000, cacheReadTokens: 800_000, cacheWriteTokens: 50_000, estimatedCostUsd: over.cost },
        orders: over.orders,
      },
    },
  },
})

const orders = (currencyCode: string, orderCount: number, assistantOrders: number, assistantRevenue: number, agentOrders = 0, agentRevenue = 0) => ({
  __typename: 'AssistedOrders',
  currencyCode,
  orderCount,
  assistantOrders,
  assistantRevenue,
  agentOrders,
  agentRevenue,
})

function renderWidget(mock: ReturnType<typeof report>) {
  render(
    <MemoryRouter>
      <MockedProvider mocks={[mock]}>
        <AssistantWidget />
      </MockedProvider>
    </MemoryRouter>,
  )
}

describe('AssistantWidget', () => {
  it('puts the cost beside what the assistant sold', async () => {
    renderWidget(report({ cost: 6, orders: [orders('EUR', 30, 3, 45000, 1, 9000), orders('USD', 10, 1, 2000)] }))
    expect(await screen.findByText('40')).toBeInTheDocument()
    expect(screen.getByText('2 cut short')).toBeInTheDocument()
    expect(screen.getByText('5 sent to checkout')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
    expect(screen.getByText(/\+1 more · 10% of orders$/)).toBeInTheDocument()
    expect(screen.getByText('$6.00')).toBeInTheDocument()
    expect(screen.getByText('$1.50 per assisted order · 1M tokens · 80% from cache')).toBeInTheDocument()
    expect(screen.getByText('15% of messages')).toBeInTheDocument()
  })

  it('says what it needs before it can price anything', async () => {
    renderWidget(report({ cost: null, orders: [] }))
    expect(await screen.findByText('1M tokens')).toBeInTheDocument()
    expect(screen.getByText('80% from cache · add prices for a cost')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Model use/ })).toHaveAttribute('href', '/settings/ai/usage')
    expect(screen.getByText('none yet')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Overview/ })).toHaveAttribute('href', '/agentic')
  })
})
