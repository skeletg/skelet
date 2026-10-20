import type { ReactNode } from 'react'
import { screen } from '@testing-library/react'
import type { DocumentNode } from 'graphql'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { BotTrafficDocument, LensErrorsDocument, LensOverviewDocument, LensPerformanceDocument, LensSearchAnalyticsDocument } from './Lens.gql'
import { LensBots } from './LensBots'
import { LensErrors } from './LensErrors'
import { LensHome } from './LensHome'
import { LensPerformance } from './LensPerformance'
import { LensSearch } from './LensSearch'
import { ExperimentsOverviewDocument } from '@/features/prism/Prism.gql'
import { PrismHome } from '@/features/prism/PrismHome'

// What these pin: a failed load shows the error with a Retry - these pages used to render "No data.", zeroed tiles or
// nothing at all, which reads as a quiet store rather than a broken request

const channelScope = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'c1', locale: '' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'c1',
  selectedChannel: { id: 'c1', name: 'Main', slug: 'main' },
} as unknown as ConfigScopeContextValue

const pages: Array<[string, ReactNode, DocumentNode]> = [
  ['LensHome', <LensHome />, LensOverviewDocument],
  ['LensBots', <LensBots />, BotTrafficDocument],
  ['LensErrors', <LensErrors />, LensErrorsDocument],
  ['LensPerformance', <LensPerformance />, LensPerformanceDocument],
  ['LensSearch', <LensSearch />, LensSearchAnalyticsDocument],
  ['PrismHome', <PrismHome />, ExperimentsOverviewDocument],
]

describe('analytics pages on a failed load', () => {
  it.each(pages)('%s shows the error and a Retry', async (_name, page, query) => {
    renderPage({
      path: '/x',
      routes: { '/x': page },
      adminId: 'admin-1',
      scope: channelScope,
      mocks: [meMock(), { request: { query, variables: () => true }, error: new Error('analytics are down'), maxUsageCount: 3 }],
    })
    expect(await screen.findByText('analytics are down', {}, SLOW)).toBeTruthy()
    expect(screen.getByRole('button', { name: /Retry/ })).toBeTruthy()
  })
})
