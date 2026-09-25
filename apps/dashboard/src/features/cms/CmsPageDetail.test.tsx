import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, globalScope, meMock, renderPage } from '@/test/page-harness'
import { CmsPageDetail } from './CmsPageDetail'
import { AiStatusDocument, CmsPageDocument, UpsertCmsPageContentDocument } from './Cms.gql'

// What these pin: the locale's content is staged in the page's review and written as one content row
// carrying the block layout, and a blank title is refused before it reaches the service

const page = {
  __typename: 'CMSPage' as const,
  experimentKey: null,
  variants: [],
  id: 'pg1',
  pageTypeId: null,
  parentId: null,
  slug: 'about',
  urlPath: 'about',
  position: 0,
  isPublished: false,
  seoNoindex: false,
  publishedAt: null,
  scheduledAt: null,
  data: {},
  tags: [],
  metadata: [],
  workflowState: 'DRAFT',
  assignedTo: null,
  submittedBy: null,
  submittedAt: null,
  approvedBy: null,
  approvedAt: null,
  content: { __typename: 'CMSPageContent' as const, locale: 'en', title: 'About', body: null, metaTitle: null, metaDescription: null, metaKeywords: null, data: {} },
  allContent: [
    {
      __typename: 'CMSPageContent' as const,
      locale: 'en',
      title: 'About',
      body: { version: 1, blocks: [] },
      metaTitle: null,
      metaDescription: null,
      metaKeywords: null,
      data: {},
      updatedAt: '2026-01-01T00:00:00Z',
    },
  ],
  selections: [],
  relationships: [],
  channelListings: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

function renderCmsPage() {
  const onContent = vi.fn()
  renderPage({
    path: '/cms/pages/pg1',
    adminId: 'admin-1',
    scope: { ...globalScope, effectiveLocale: 'en' },
    routes: { '/cms/pages/:id': <CmsPageDetail /> },
    mocks: [
      meMock(),
      { request: { query: CmsPageDocument, variables: { id: 'pg1' } }, result: { data: { cmsPage: page } }, maxUsageCount: 5 },
      { request: { query: AiStatusDocument }, result: { data: { aiStatus: { __typename: 'AiStatus', enabled: false } } }, maxUsageCount: 3 },
      { request: { query: UpsertCmsPageContentDocument, variables: captureInput(onContent) }, result: { data: { upsertCMSPageContent: page } } },
    ],
  })
  return onContent
}

afterEach(() => localStorage.clear())

describe('CmsPageDetail', () => {
  it('writes the edited content row with its layout', async () => {
    const onContent = renderCmsPage()
    fireEvent.change(await screen.findByLabelText('Meta title', {}, SLOW), { target: { value: 'About us' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(
      () =>
        expect(onContent).toHaveBeenCalledWith(
          expect.objectContaining({ pageId: 'pg1', locale: 'en', title: 'About', metaTitle: 'About us', body: { version: 1, blocks: [] } }),
        ),
      SLOW,
    )
  })

  it('refuses a blank title', async () => {
    const onContent = renderCmsPage()
    fireEvent.change(await screen.findByLabelText('Title', {}, SLOW), { target: { value: ' ' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/Title is required/)).toBeDefined()
    expect(onContent).not.toHaveBeenCalled()
  })
})
