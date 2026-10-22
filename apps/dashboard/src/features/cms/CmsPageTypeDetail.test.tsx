import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { CmsPageTypeDetail } from './CmsPageTypeDetail'
import {
  CmsAttributeGroupsDocument,
  CmsAttributesDocument,
  CmsPagesDocument,
  CmsPageTypesDocument,
  DeleteCmsPageTypeDocument,
  SetCmsPageTypeDefaultBlocksDocument,
  UpdateCmsPageTypeDocument,
  UpdateCmsPageTypeStructureDocument,
  UpsertCmsPageTypeContentDocument,
} from './Cms.gql'

// What these pin: the four former panels are one review, a URL root the backend would refuse is
// caught first, and each write goes only when its part changed

const pageType = {
  __typename: 'CMSPageType' as const,
  id: 'pt1',
  slug: 'help',
  icon: null,
  isSystem: false,
  position: 0,
  rootSlug: null,
  channelRoots: [],
  defaultBlocks: null,
  defaultMetaTitle: null,
  defaultSeoNoindex: false,
  isSingleton: false,
  pageCount: 0,
  content: { __typename: 'CMSPageTypeContent' as const, locale: 'en', name: 'Help', description: null },
  allContent: [{ __typename: 'CMSPageTypeContent' as const, locale: 'en', name: 'Help', description: null, updatedAt: '2026-01-01T00:00:00Z' }],
  attributes: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

afterEach(() => localStorage.clear())

describe('CmsPageTypeDetail', () => {
  it('refuses a root with spaces, then writes only the naming and structure', async () => {
    const writes = { content: vi.fn(), structure: vi.fn(), display: vi.fn(), blocks: vi.fn() }
    renderPage({
      path: '/cms/page-types/pt1',
      adminId: 'admin-1',
      routes: { '/cms/page-types/:id': <CmsPageTypeDetail /> },
      mocks: [
        meMock(),
        { request: { query: CmsPageTypesDocument }, result: { data: { cmsPageTypes: [pageType] } }, maxUsageCount: 5 },
        { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 3 },
        { request: { query: CmsPagesDocument, variables: () => true }, result: { data: { cmsPages: [] } }, maxUsageCount: 3 },
        { request: { query: CmsAttributesDocument }, result: { data: { cmsAttributes: [] } }, maxUsageCount: 3 },
        { request: { query: CmsAttributeGroupsDocument, variables: () => true }, result: { data: { cmsAttributeGroups: [] } }, maxUsageCount: 3 },
        {
          request: { query: UpsertCmsPageTypeContentDocument, variables: captureInput(writes.content) },
          result: { data: { upsertCMSPageTypeContent: pageType } },
        },
        {
          request: { query: UpdateCmsPageTypeStructureDocument, variables: captureInput(writes.structure) },
          result: { data: { updateCMSPageTypeStructure: pageType } },
        },
        { request: { query: UpdateCmsPageTypeDocument, variables: recorder(writes.display) }, result: { data: { updateCMSPageType: pageType } } },
        {
          request: { query: SetCmsPageTypeDefaultBlocksDocument, variables: recorder(writes.blocks) },
          result: { data: { setCMSPageTypeDefaultBlocks: pageType } },
        },
      ],
    })

    const save = await screen.findByRole('button', { name: 'Save changes' })
    fireEvent.change(screen.getByLabelText('Description (en)'), { target: { value: 'Answers for customers' } })
    fireEvent.change(screen.getByLabelText('URL root'), { target: { value: 'help center' } })
    fireEvent.click(save)
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/segments split by/)).toBeDefined()
    expect(writes.structure).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    fireEvent.change(screen.getByLabelText('URL root'), { target: { value: '/help/' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Apply 2 changes/ }))

    await vi.waitFor(() => expect(writes.structure).toHaveBeenCalled(), SLOW)
    expect(writes.structure).toHaveBeenCalledWith(expect.objectContaining({ id: 'pt1', rootSlug: 'help' }))
    expect(writes.content).toHaveBeenCalledWith({ pageTypeId: 'pt1', locale: 'en', name: 'Help', description: 'Answers for customers' })
    expect(writes.display).not.toHaveBeenCalled()
    expect(writes.blocks).not.toHaveBeenCalled()
  })
})

// The delete confirm runs through the shared ConfirmDialog: a refused delete keeps it open with the server's
// message, a successful one goes back to the list
describe('CmsPageTypeDetail delete', () => {
  async function confirmDelete(mock: MockLink.MockedResponse) {
    renderPage({
      path: '/cms/page-types/pt1',
      adminId: 'admin-1',
      routes: { '/cms/page-types/:id': <CmsPageTypeDetail />, '/cms/page-types': <p>Page types list</p> },
      mocks: [
        meMock(),
        { request: { query: CmsPageTypesDocument }, result: { data: { cmsPageTypes: [pageType] } }, maxUsageCount: 5 },
        { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 3 },
        { request: { query: CmsPagesDocument, variables: () => true }, result: { data: { cmsPages: [] } }, maxUsageCount: 3 },
        { request: { query: CmsAttributesDocument }, result: { data: { cmsAttributes: [] } }, maxUsageCount: 3 },
        { request: { query: CmsAttributeGroupsDocument, variables: () => true }, result: { data: { cmsAttributeGroups: [] } }, maxUsageCount: 3 },
        mock,
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete page type' }))
    return dialog
  }

  it('keeps the dialog open and shows a refused delete', async () => {
    const dialog = await confirmDelete({ request: { query: DeleteCmsPageTypeDocument, variables: { id: 'pt1' } }, result: { errors: [{ message: 'page type has pages' }] } })
    expect(await within(dialog).findByText('page type has pages')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteCmsPageTypeDocument, variables: { id: 'pt1' } }, result: { data: { deleteCMSPageType: true } } })
    expect(await screen.findByText('Page types list', {}, SLOW)).toBeDefined()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
