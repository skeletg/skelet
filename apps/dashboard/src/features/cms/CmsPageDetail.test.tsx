import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, globalScope, meMock, renderPage } from '@/test/page-harness'
import { CmsPageDetail } from './CmsPageDetail'
import {
  AiStatusDocument,
  CmsPageCommentsDocument,
  CmsPageDocument,
  DeleteCmsPageDocument,
  UpdateCmsPageDocument,
  UpsertCmsPageContentDocument,
} from './Cms.gql'

// What these pin: the locale's content is staged in the page's review and written as one content row
// carrying the block layout, a blank title is refused before it reaches the service, search indexing has
// one owner, and global scope points at a scope that can edit

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

function renderCmsPage(extra: MockLink.MockedResponse[] = [], scope = { ...globalScope, effectiveLocale: 'en' }) {
  const onContent = vi.fn()
  renderPage({
    path: '/cms/pages/pg1',
    adminId: 'admin-1',
    scope,
    routes: { '/cms/pages/:id': <CmsPageDetail />, '/cms/pages': <p>Pages list</p> },
    mocks: [
      ...extra,
      meMock(),
      { request: { query: CmsPageDocument, variables: { id: 'pg1' } }, result: { data: { cmsPage: page } }, maxUsageCount: 5 },
      { request: { query: AiStatusDocument }, result: { data: { aiStatus: { __typename: 'AiStatus', enabled: false } } }, maxUsageCount: 3 },
      { request: { query: CmsPageCommentsDocument, variables: { pageId: 'pg1' } }, result: { data: { cmsPageComments: [] } }, maxUsageCount: 3 },
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

  // Basics and the SEO panel used to both stage seoNoindex and write it on every save, so whichever ran
  // last won - a slug change could quietly undo the indexing switch
  it('writes search indexing from the SEO panel only', async () => {
    const onUpdate = vi.fn()
    renderCmsPage([
      { request: { query: UpdateCmsPageDocument, variables: captureInput(onUpdate) }, result: { data: { updateCMSPage: page } }, maxUsageCount: 2 },
    ])
    fireEvent.change(await screen.findByLabelText('Slug', {}, SLOW), { target: { value: 'about-us' } })
    fireEvent.click(screen.getByRole('button', { name: /Previews, indexing and structured data/ }))
    fireEvent.click(screen.getByRole('switch', { name: 'Allow search indexing' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(2), SLOW)
    expect(onUpdate).toHaveBeenCalledWith({ id: 'pg1', slug: 'about-us', data: {} })
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: 'pg1', seoNoindex: true }))
  })

  it('shows global scope read-only and offers a scope that can edit', async () => {
    const setScope = vi.fn(() => true)
    const europe = { id: 'c1', name: 'Europe', defaultLocale: 'en', locales: [{ code: 'en' }], isActive: true }
    renderCmsPage([], { ...globalScope, effectiveLocale: '', channels: [europe], setScope })
    // Disabled through its fieldset, which the input's own disabled property doesn't reflect
    expect((await screen.findByLabelText('Title', {}, SLOW)).matches(':disabled')).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Edit in Europe · en' }))
    expect(setScope).toHaveBeenCalledWith({ mode: 'CHANNEL', channelId: 'c1', locale: 'en' })
  })
})

// The delete confirm runs through the shared ConfirmDialog and waits for the server: a refused delete keeps it open
// with the server's message (it used to close and drop it), a successful one goes back to the list
describe('CmsPageDetail delete', () => {
  async function confirmDelete(mock: MockLink.MockedResponse) {
    renderCmsPage([mock])
    fireEvent.pointerDown(await screen.findByRole('button', { name: 'More actions' }, SLOW), { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
    return dialog
  }

  it('keeps the dialog open and shows a refused delete', async () => {
    const dialog = await confirmDelete({ request: { query: DeleteCmsPageDocument, variables: { id: 'pg1' } }, result: { errors: [{ message: 'page is the home page' }] } })
    expect(await within(dialog).findByText('page is the home page')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteCmsPageDocument, variables: { id: 'pg1' } }, result: { data: { deleteCMSPage: true } } })
    expect(await screen.findByText('Pages list', {}, SLOW)).toBeDefined()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
