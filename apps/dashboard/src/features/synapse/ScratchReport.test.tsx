import { fireEvent, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Route, Routes, useLocation } from 'react-router-dom'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { SynapseInsightsDocument, SynapseOverviewDocument } from './Synapse.gql'
import { SynapseInsights } from './SynapseInsights'
import { readReport } from './ScratchReport'

const channelScope = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'c1', locale: '' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'c1',
  selectedChannel: { id: 'c1', name: 'Main', slug: 'main' },
} as unknown as ConfigScopeContextValue

const insights = { __typename: 'SynapseInsights', points: [], series: [], uniques: 0, prevUniques: 0, aggregate: 0, totalAmount: 0, prevTotalAmount: 0, baseCurrency: 'USD' }

function Search() {
  return <output data-testid="search">{useLocation().search}</output>
}

describe('readReport', () => {
  it("falls back to the page's default for a missing, broken or foreign report", () => {
    const p = (q: string) => new URLSearchParams({ q })
    expect(readReport(new URLSearchParams(), 'INSIGHTS').kind).toBe('INSIGHTS')
    expect(readReport(p('{oops'), 'INSIGHTS').kind).toBe('INSIGHTS')
    expect(readReport(p('{"kind":"FUNNEL","steps":["a","b"]}'), 'INSIGHTS').kind).toBe('INSIGHTS')
    expect(readReport(p('{"kind":"INSIGHTS","event":"cart_add"}'), 'INSIGHTS').event).toBe('cart_add')
    expect(readReport(p('{"kind":"FUNNEL","steps":"page_view"}'), 'FUNNEL').steps).toHaveLength(5)
    expect(readReport(p('{"kind":"FUNNEL","steps":[1,2]}'), 'FUNNEL').steps).toHaveLength(5)
    expect(readReport(p('{"kind":"INSIGHTS","event":5}'), 'INSIGHTS').event).toBe('page_view')
  })
  it("opens a view of the page's builder, and the page's start when there's no report", () => {
    const p = (q: string) => new URLSearchParams({ q })
    expect(readReport(p('{"kind":"STICKINESS"}'), 'RETENTION').kind).toBe('STICKINESS')
    expect(readReport(new URLSearchParams(), 'RETENTION', 'STICKINESS').kind).toBe('STICKINESS')
  })
})

describe('a report page', () => {
  it('opens the report and period in its URL, and writes each change back to it', async () => {
    const asked: Record<string, unknown>[] = []
    const q = encodeURIComponent(JSON.stringify({ kind: 'INSIGHTS', event: 'cart_add' }))
    renderPage({
      path: `/x?q=${q}&days=90`,
      routes: {
        '/x': (
          <>
            <SynapseInsights />
            <Routes>
              <Route path="*" element={<Search />} />
            </Routes>
          </>
        ),
      },
      scope: channelScope,
      mocks: [
        meMock({ superuser: true }),
        { request: { query: SynapseOverviewDocument, variables: () => true }, error: new Error('none'), maxUsageCount: 5 },
        {
          request: {
            query: SynapseInsightsDocument,
            variables: (v: Record<string, unknown>) => {
              asked.push(v)
              return true
            },
          },
          result: { data: { synapseInsights: insights } },
          maxUsageCount: 5,
        },
      ],
    })
    await waitFor(() => expect(asked[0]).toMatchObject({ event: 'cart_add', sinceDays: 90 }), SLOW)
    fireEvent.click(screen.getByRole('button', { name: 'Unique' }))
    await waitFor(() => expect(asked.at(-1)).toMatchObject({ event: 'cart_add', measure: 'unique', sinceDays: 90 }), SLOW)
    const search = new URLSearchParams(screen.getByTestId('search').textContent ?? '')
    expect(JSON.parse(search.get('q') ?? '')).toMatchObject({ kind: 'INSIGHTS', event: 'cart_add', measure: 'unique' })
    expect(search.get('days')).toBe('90')
  })

  it('offers the shops right in its empty state on global scope', async () => {
    const setScope = vi.fn(() => true)
    renderPage({
      path: '/x',
      routes: { '/x': <SynapseInsights /> },
      scope: { ...globalScope, setScope, channels: [{ id: 'c1', name: 'Main', defaultLocale: 'en', locales: [], isActive: true }] },
      mocks: [meMock({ superuser: true })],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Main' }, SLOW))
    expect(setScope).toHaveBeenCalledWith({ mode: 'CHANNEL', channelId: 'c1', locale: '' })
  })
})
