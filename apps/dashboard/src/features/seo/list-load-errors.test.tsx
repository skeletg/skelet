import type { ReactNode } from 'react'
import { screen } from '@testing-library/react'
import type { DocumentNode } from 'graphql'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CmsBlockTypesDocument } from '@/features/cms/Cms.gql'
import { CmsBlockTypesPage } from '@/features/cms/CmsBlockTypesPage'
import { ProductQuestionsDocument } from '@/features/qna/Qna.gql'
import { QuestionsList } from '@/features/qna/QuestionsList'
import { CrawlAuditDocument } from './LinkMap.gql'
import { CrawlAuditTable } from './CrawlAuditTable'

// What these pin: a failed list load shows the error in place of the table - it used to read as an empty list

const lists: Array<[string, ReactNode, DocumentNode]> = [
  ['CrawlAuditTable', <CrawlAuditTable channelId="c1" />, CrawlAuditDocument],
  ['CmsBlockTypesPage', <CmsBlockTypesPage />, CmsBlockTypesDocument],
  ['QuestionsList', <QuestionsList />, ProductQuestionsDocument],
]

describe('lists on a failed load', () => {
  it.each(lists)('%s shows the error instead of an empty table', async (_name, list, query) => {
    renderPage({
      path: '/x',
      routes: { '/x': list },
      adminId: 'admin-1',
      mocks: [meMock(), { request: { query, variables: () => true }, error: new Error('list is down'), maxUsageCount: 3 }],
    })
    expect(await screen.findByText('list is down', {}, SLOW)).toBeTruthy()
  })
})
