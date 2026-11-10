import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { ServerFeaturesDocument } from '@/shared/features/Features.gql'
import { globalScope, meMock, renderPage } from '@/test/page-harness'
import { AgenticReportDocument, AgenticStatusDocument, AssistantNameDocument } from './Agentic.gql'
import { AgenticOverview } from './AgenticOverview'

const store = { ...globalScope, scope: { mode: 'CHANNEL' as const, channelId: 'ch1', locale: '' }, isGlobal: false, isChannel: true, effectiveChannelId: 'ch1', selectedChannel: { id: 'ch1', name: 'Main' } } as typeof globalScope

const day = (date: string, turns: number, assistantOrders = 0) => ({
  __typename: 'AssistantDay',
  day: date,
  turns,
  failedTurns: 0,
  unansweredTurns: 0,
  cartTurns: Math.floor(turns / 2),
  checkoutTurns: 0,
  assistantOrders,
  agentOrders: 0,
})

const report = {
  request: { query: AgenticReportDocument, variables: { channelId: 'ch1', days: 30 } },
  result: {
    data: {
      assistantReport: {
        __typename: 'AssistantReport',
        days: 30,
        turns: 42,
        failedTurns: 0,
        unansweredTurns: 7,
        cartTurns: 21,
        checkoutTurns: 3,
        searchOverviews: 12,
        usage: { __typename: 'AssistantUsage', inputTokens: 1000, outputTokens: 500, cacheReadTokens: 0, cacheWriteTokens: 0, estimatedCostUsd: null },
        orders: [{ __typename: 'AssistedOrders', currencyCode: 'USD', orderCount: 10, assistantOrders: 2, assistantRevenue: 9900, agentOrders: 0, agentRevenue: 0 }],
        daily: [day('2026-11-03', 0), day('2026-11-04', 40, 2), day('2026-11-05', 2)],
      },
    },
  },
}

const row = (key: string, label: string, state: string, detail: string, settingsPath: string) => ({ __typename: 'AgenticStatus', key, label, state, detail, settingsPath })

const status = (result: object) => ({ request: { query: AgenticStatusDocument, variables: { channelId: 'ch1' } }, ...result })

const features = { request: { query: ServerFeaturesDocument }, maxUsageCount: 5, result: { data: { serverFeatures: { __typename: 'ServerFeatures', signals: true, ai: true } } } }

const assistantName = {
  request: { query: AssistantNameDocument, variables: { scope: { kind: 'CHANNEL', channelId: 'ch1' } } },
  result: { data: { configValue: { __typename: 'ConfigValue', path: 'sales/assistant/name', value: '' } } },
}

describe('AgenticOverview', () => {
  it("shows the store's numbers, day by day, and what's switched on with where to set it", async () => {
    renderPage({
      path: '/agentic',
      routes: { '/agentic': <AgenticOverview /> },
      mocks: [
        meMock(),
        features,
        assistantName,
        report,
        status({
          result: {
            data: {
              agenticStatus: [
                row('provider', 'AI provider', 'ON', 'claude-opus-5-5', '/settings/ai/llm'),
                row('assistant', 'Shopping assistant', 'ATTENTION', 'Switched on, but the store has no public URL', '/channels/ch1'),
                row('embeddings', 'Search by meaning', 'OFF', 'No embedding model', '/settings/ai/embeddings'),
                row('transcripts', 'Keeping conversations', 'ON', 'Kept for shoppers who are signed in', '/conversations'),
              ],
            },
          },
        }),
      ],
      scope: store,
    })
    expect(await screen.findByText('42')).toBeInTheDocument()
    expect(screen.getByText('The shopping assistant answered 42 messages in the last 30 days.')).toBeInTheDocument()
    expect(screen.getByText('21 filled a cart, 2 led to an order, and 7 went unanswered.')).toBeInTheDocument()
    expect(screen.getByText(/\$99\.00 · 20% of orders/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Couldn't answer/ })).toHaveAttribute('href', '/conversations?unanswered=any')
    expect(screen.getByText('17% of messages')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'AI overviews' })).toHaveAttribute('href', '/agentic/hybrid-search')
    expect(screen.getByText(/above search results - counted in the model use, not in the messages/)).toHaveTextContent(/^It also wrote 12 AI overviews/)
    expect(screen.getByRole('link', { name: 'AI insights' })).toHaveAttribute('href', '/ai-insights')

    expect(await screen.findByText(/2 of 4 on, 1 needing attention/)).toBeInTheDocument()
    const setup = await screen.findByText('Shopping assistant')
    const item = setup.closest('li')!
    expect(within(item).getByText('Needs attention')).toBeInTheDocument()
    expect(within(item).getByRole('link')).toHaveAttribute('href', '/channels/ch1')
    expect(within(screen.getByText('AI provider').closest('li')!).getByText('On')).toBeInTheDocument()
    expect(within(screen.getByText('Search by meaning').closest('li')!).getByText('Off')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Day by day'))
    const table = screen.getByRole('table')
    const rows = within(table).getAllByRole('row')
    expect(rows).toHaveLength(3) // the header, then the two days with something in them, newest first
    expect(within(rows[1]).getByText('Nov 5')).toBeInTheDocument()
    expect(within(rows[2]).getAllByText('2')).not.toHaveLength(0)
  })

  it("shows a reports-only admin the numbers, and leaves out the setup and conversations they can't open", async () => {
    const asked = vi.fn()
    renderPage({
      path: '/agentic',
      routes: { '/agentic': <AgenticOverview /> },
      mocks: [
        meMock({ superuser: false, permissions: ['reports.view'] }),
        features,
        report,
        {
          request: {
            query: AgenticStatusDocument,
            variables: () => {
              asked()
              return true
            },
          },
          result: { data: { agenticStatus: [] } },
        },
      ],
      scope: store,
    })
    expect(await screen.findByText('42')).toBeInTheDocument()
    expect(screen.getByText('The shopping assistant answered 42 messages in the last 30 days.')).toBeInTheDocument()
    expect(screen.queryByText("What's switched on")).toBeNull()
    expect(screen.queryByText(/Couldn't load/)).toBeNull()
    expect(screen.queryByRole('link', { name: /Couldn't answer/ })).toBeNull()
    expect(screen.getByText(/above search results - counted in the model use/)).toHaveTextContent(/^It also wrote 12 AI overviews/)
    expect(screen.queryByRole('link', { name: 'AI overviews' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'AI insights' })).toBeNull()
    expect(asked).not.toHaveBeenCalled()
  })

  it('asks for a store first', async () => {
    renderPage({ path: '/agentic', routes: { '/agentic': <AgenticOverview /> }, mocks: [meMock(), features], scope: globalScope })
    expect(await screen.findByText('Pick a store')).toBeInTheDocument()
  })
})
