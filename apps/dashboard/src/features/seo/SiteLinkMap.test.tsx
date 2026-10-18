import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { LinkMapDocument, SiteCrawlsDocument } from './LinkMap.gql'
import { SiteLinkMap } from './SiteLinkMap'

// What this pins: a running crawl is polled until it finishes and the map is pulled once when it does - through
// pollInterval now, not a hand-rolled timer that restarted on every render

const channelScope = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'c1', locale: '' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'c1',
  selectedChannel: { id: 'c1', name: 'Main', slug: 'main' },
} as unknown as ConfigScopeContextValue

const crawl = (status: string) => ({
  __typename: 'SiteCrawl',
  id: 'cr1',
  baseUrl: 'https://shop.example',
  status,
  maxPages: 100,
  maxDepth: 5,
  respectRobots: true,
  pagesCrawled: 10,
  linksFound: 20,
  hitBudget: false,
  error: null,
  startedAt: '2026-01-01T00:00:00Z',
  finishedAt: status === 'RUNNING' ? null : '2026-01-01T00:01:00Z',
})

const graph = {
  __typename: 'LinkGraph',
  nodes: [],
  edges: [],
  stats: {
    __typename: 'LinkGraphStats',
    totalNodes: 0, totalEdges: 0, crawled: 0, discovered: 0, unseen: 0, orphans: 0, noindexed: 0, broken: 0,
    redirected: 0, boilerplate: 0, contextual: 0, maxDepth: 0, returnedNodes: 0, dropped: 0, complete: true,
  },
  crawl: { __typename: 'SiteCrawl', id: 'cr1', baseUrl: 'https://shop.example', status: 'DONE', pagesCrawled: 10, linksFound: 20, hitBudget: false },
}

afterEach(() => vi.useRealTimers())

describe('SiteLinkMap crawl polling', () => {
  it('polls a running crawl and pulls the map once when it finishes', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const crawlAsks = vi.fn()
    const mapAsks = vi.fn()
    const crawls = (status: string) => ({
      request: {
        query: SiteCrawlsDocument,
        variables: () => {
          crawlAsks()
          return true
        },
      },
      result: { data: { siteCrawls: [crawl(status)] } },
    })
    renderPage({
      path: '/seo/link-map',
      adminId: 'admin-1',
      scope: channelScope,
      routes: { '/seo/link-map': <SiteLinkMap /> },
      mocks: [
        meMock(),
        crawls('RUNNING'),
        crawls('RUNNING'),
        { ...crawls('DONE'), maxUsageCount: 10 },
        {
          request: {
            query: LinkMapDocument,
            variables: () => {
              mapAsks()
              return true
            },
          },
          result: { data: { linkGraph: graph } },
          maxUsageCount: 10,
        },
      ],
    })
    await vi.waitFor(() => expect(crawlAsks).toHaveBeenCalledTimes(1), SLOW)
    expect(mapAsks).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(7_000)
    await vi.waitFor(() => expect(crawlAsks).toHaveBeenCalledTimes(3), SLOW)
    await vi.waitFor(() => expect(mapAsks).toHaveBeenCalledTimes(2), SLOW)
    // finished - no more polling
    await vi.advanceTimersByTimeAsync(10_000)
    expect(crawlAsks).toHaveBeenCalledTimes(3)
  })
})
