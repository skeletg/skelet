import { screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import { AiInsightsDocument } from './AIInsights.gql'
import { AIInsights } from './AIInsights'

const store = { ...globalScope, scope: { mode: 'CHANNEL' as const, channelId: 'ch1', locale: '' }, isGlobal: false, isChannel: true, effectiveChannelId: 'ch1', selectedChannel: { id: 'ch1', name: 'Main' } } as typeof globalScope

const cohort = (people: number, buyers: number, orders: number, revenue: number, returnedOrders: number) => ({
  __typename: 'AICohort',
  people,
  buyers,
  orders,
  revenue,
  revenueOrders: orders,
  returnedOrders,
})

const insights = (over: Record<string, unknown> = {}) => ({
  request: { query: AiInsightsDocument, variables: { channelId: 'ch1', days: 30 } },
  result: {
    data: {
      aiInsights: {
        __typename: 'AIInsights',
        days: 30,
        cartWindowDays: 7,
        orderWindowDays: 14,
        currencyCode: 'USD',
        chatted: cohort(200, 20, 25, 250000, 1),
        didNotChat: cohort(4000, 120, 130, 910000, 13),
        products: [
          { __typename: 'AIRecommendedProduct', productId: 'p1', name: 'Linen Shirt Dress', shown: 40, people: 30, carted: 9, ordered: 3 },
          { __typename: 'AIRecommendedProduct', productId: 'p2', name: null, shown: 2, people: 2, carted: 0, ordered: 0 },
        ],
        agents: [
          {
            __typename: 'AIAgentActivity',
            agent: 'Claude',
            calls: 120,
            searches: 40,
            emptySearches: 6,
            tools: [
              { __typename: 'AIToolCount', tool: 'search_products', calls: 40 },
              { __typename: 'AIToolCount', tool: 'add_to_cart', calls: 12 },
            ],
          },
        ],
        agentOrders: 4,
        agentRevenue: 48000,
        ...over,
      },
    },
  },
})

const render = (mocks: Parameters<typeof renderPage>[0]['mocks']) =>
  renderPage({ path: '/ai-insights', routes: { '/ai-insights': <AIInsights /> }, adminId: 'admin-1', mocks, scope: store })

describe('AIInsights', () => {
  it('compares shoppers who chatted with the rest, and lists what it shows and the agents', async () => {
    render([meMock(), insights()])
    const headline = await screen.findByText((_, el) => el?.tagName === 'P' && /^Shoppers who chatted/.test(el.textContent ?? ''), {}, SLOW)
    expect(headline).toHaveTextContent("Shoppers who chatted converted at 10%, against 3.0% for those who didn't.")
    const converted = screen.getByRole('row', { name: /Converted/ })
    expect(within(converted).getByText('+7.0 pts')).toHaveClass('text-success')
    const aov = screen.getByRole('row', { name: /Average order/ })
    expect(within(aov).getByText('$100.00')).toBeInTheDocument()
    expect(within(aov).getByText('$70.00')).toBeInTheDocument()
    expect(within(aov).getByText('+43%')).toHaveClass('text-success')
    // fewer returns among chatters reads as the better side
    expect(within(screen.getByRole('row', { name: /Returned/ })).getByText('-6.0 pts')).toHaveClass('text-success')

    expect(screen.getByRole('link', { name: 'Linen Shirt Dress' })).toHaveAttribute('href', '/products/p1')
    expect(screen.getByText('Removed product')).toBeInTheDocument()
    expect(screen.getByText('30%')).toBeInTheDocument()

    expect(screen.getByText('Claude')).toBeInTheDocument()
    expect(screen.getByText('search_products (40), add_to_cart (12)')).toBeInTheDocument()
    expect(screen.getByText('$480.00')).toBeInTheDocument()
  })

  it('says so when nobody chatted and nothing was shown', async () => {
    render([
      meMock(),
      insights({ chatted: cohort(0, 0, 0, 0, 0), products: [], agents: [], agentOrders: 0, agentRevenue: 0 }),
    ])
    expect(await screen.findByText(/Nobody chatted with the assistant in this window/, {}, SLOW)).toBeInTheDocument()
    expect(screen.getByText("The assistant hasn't shown any products in this window.")).toBeInTheDocument()
    expect(screen.getByText("No shopper's agent has used the store in this window.")).toBeInTheDocument()
  })

  it("tells an admin who can't view the assistant here, without asking for the numbers", async () => {
    const asked = vi.fn()
    render([
      meMock({ superuser: false, permissions: ['reports.view'] }),
      {
        request: {
          query: AiInsightsDocument,
          variables: () => {
            asked()
            return true
          },
        },
        result: { data: { aiInsights: null } },
      },
    ])
    expect(await screen.findByText('Not on this store')).toBeInTheDocument()
    expect(asked).not.toHaveBeenCalled()
  })

  it('asks for a store first', async () => {
    renderPage({ path: '/ai-insights', routes: { '/ai-insights': <AIInsights /> }, mocks: [meMock()], scope: globalScope })
    expect(await screen.findByText('Pick a store')).toBeInTheDocument()
  })
})
