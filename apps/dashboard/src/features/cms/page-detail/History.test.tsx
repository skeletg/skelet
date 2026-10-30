import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { CmsPageRevisionsDocument } from '../Cms.gql'
import { CmsPageHistory } from './History'

// What this pins: each version says what it changed against the one saved before it, so a list of
// versions sharing one title can still be told apart

const doc = (heading: string) => ({ version: 1, blocks: [{ id: 'b1', type: 'hero', props: { heading } }] })
const revision = (id: string, title: string, heading: string, createdAt: string) => ({
  __typename: 'CMSPageRevision' as const,
  id,
  pageId: 'pg1',
  locale: 'en',
  title,
  body: doc(heading),
  metaTitle: null,
  metaDescription: null,
  metaKeywords: null,
  authorAdminId: null,
  createdAt,
})

describe('CmsPageHistory', () => {
  it('summarises each version against the one before it', async () => {
    renderDialog(<CmsPageHistory pageId="pg1" locale="en" canManage refetchVariables={{ id: 'pg1' }} />, [
      {
        request: { query: CmsPageRevisionsDocument, variables: { pageId: 'pg1', locale: 'en' } },
        result: {
          data: {
            cmsPageRevisions: [
              revision('r3', 'About us', 'Hello', '2026-10-03T10:00:00Z'),
              revision('r2', 'About', 'Hi', '2026-10-02T10:00:00Z'),
              revision('r1', 'About', 'Hi', '2026-10-01T10:00:00Z'),
            ],
          },
        },
      },
    ])
    expect(await screen.findByText('Title, 1 block change', {}, SLOW)).toBeDefined()
    expect(screen.getByText('No content changes')).toBeDefined()
    expect(screen.getByText('Oldest kept version')).toBeDefined()
  })
})
