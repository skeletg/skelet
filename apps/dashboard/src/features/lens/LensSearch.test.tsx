import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { LensSearchAnalyticsDocument } from './Lens.gql'
import { LensSearch } from './LensSearch'

const channelScope = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'c1', locale: '' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'c1',
  selectedChannel: { id: 'c1', name: 'Main', slug: 'main' },
} as unknown as ConfigScopeContextValue

const term = (t: string, count: number) => ({ __typename: 'LensSearchTerm' as const, term: t, count })

const analytics = (over: Record<string, unknown> = {}) => ({
  __typename: 'LensSearchAnalytics',
  totalSearches: 40,
  distinctTerms: 12,
  zeroResultSearches: 3,
  top: [term('linen shirt', 20)],
  zeroResult: [term('zxqv', 3)],
  meaningOnlySearches: 7,
  meaningOnly: [term('something breezy for a wedding', 7)],
  daily: [],
  aiGaps: [],
  overviewsShown: 0,
  overviewClicks: 0,
  overviewChats: 0,
  overviews: [],
  ...over,
})

const render = (data: ReturnType<typeof analytics>) =>
  renderPage({
    path: '/lens/search',
    routes: { '/lens/search': <LensSearch /> },
    adminId: 'admin-1',
    scope: channelScope,
    mocks: [
      meMock(),
      {
        request: { query: LensSearchAnalyticsDocument, variables: { channelId: 'c1', sinceDays: 30 } },
        result: { data: { lensSearchAnalytics: data } },
      },
    ],
  })

describe('LensSearch', () => {
  it('lists the terms only meaning found anything for apart from the ones that found nothing', async () => {
    render(analytics())
    expect(await screen.findByText('something breezy for a wedding', {}, SLOW)).toBeTruthy()
    expect(screen.getByText(/^7 searches whose words matched no product/)).toBeTruthy()
    expect(screen.getByText('zxqv')).toBeTruthy()
  })

  it('shows what the AI was asked for and found nothing, and how the overviews did', async () => {
    render(
      analytics({
        aiGaps: [{ __typename: 'LensAISearchGap', term: 'wide fit waterproof boots', assistant: 4, agents: 2 }],
        overviewsShown: 200,
        overviewClicks: 30,
        overviewChats: 10,
        overviews: [{ __typename: 'LensOverviewTerm', term: 'a dress for a wedding', shown: 50, clicks: 10, chats: 5 }],
      }),
    )
    expect(await screen.findByText('wide fit waterproof boots', {}, SLOW)).toBeTruthy()
    expect(screen.getAllByText('Asked of the AI, found nothing').length).toBeGreaterThan(0)
    expect(screen.getByText('15%')).toBeTruthy()
    expect(screen.getByText('5%')).toBeTruthy()
    expect(screen.getByText('a dress for a wedding')).toBeTruthy()
    expect(screen.getByText('20%')).toBeTruthy()
    expect(screen.getByText('10%')).toBeTruthy()
  })

  it('says so when the AI has nothing to report yet', async () => {
    render(analytics())
    expect(await screen.findByText(/every search the AI made found something/, {}, SLOW)).toBeTruthy()
    expect(screen.getByText('No overviews shown yet.')).toBeTruthy()
  })
})
