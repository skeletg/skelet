import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { CmsPageRevisionsDocument, RestoreCmsPageRevisionDocument } from '../Cms.gql'
import { CmsPageHistory } from './History'

// What these pin: each version says what it changed against the one saved before it, so a list of
// versions sharing one title can still be told apart, and an older version restores from its own pane

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

const revisionsMock = {
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
  maxUsageCount: 3,
}

describe('CmsPageHistory', () => {
  it('summarises each version against the one before it', async () => {
    renderDialog(<CmsPageHistory pageId="pg1" locale="en" canManage refetchVariables={{ id: 'pg1' }} />, [revisionsMock])
    expect(await screen.findByText('Title, 1 block change', {}, SLOW)).toBeDefined()
    expect(screen.getByText('No content changes')).toBeDefined()
    expect(screen.getByText('Oldest kept version')).toBeDefined()
    expect(screen.getByText('Current version')).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Restore this version' })).toBeNull()
  })

  it('restores an older version from its pane', async () => {
    const onRestore = vi.fn()
    renderDialog(<CmsPageHistory pageId="pg1" locale="en" canManage refetchVariables={{ id: 'pg1' }} />, [
      revisionsMock,
      { request: { query: RestoreCmsPageRevisionDocument, variables: captureInput(onRestore) }, result: { data: { restoreCMSPageRevision: null } } },
    ])
    fireEvent.click(await screen.findByText('No content changes', {}, SLOW))
    expect(screen.getByText('Changed since this version')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Restore this version' }))
    await vi.waitFor(() => expect(onRestore).toHaveBeenCalledWith({ id: 'r2' }), SLOW)
  })
})
