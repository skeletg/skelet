import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { CmsPageCreate } from './CmsPageCreate'
import { CmsPageTypesDocument, CmsPagesDocument, CreateCmsPageDocument } from './Cms.gql'

const pageType = (id: string, name: string) => ({
  __typename: 'CMSPageType' as const,
  id,
  slug: id,
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
  content: { __typename: 'CMSPageTypeContent' as const, locale: 'en', name, description: null },
  allContent: [],
  attributes: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
})

describe('CmsPageCreate', () => {
  it('takes a deep-linked page type and the editing locale, then opens the new page', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/cms/pages/new?pageTypeId=landing',
      routes: { '/cms/pages/new': <CmsPageCreate />, '/cms/pages/:id': <p>page editor</p> },
      mocks: [
        { request: { query: CmsPageTypesDocument }, result: { data: { cmsPageTypes: [pageType('landing', 'Landing')] } } },
        { request: { query: LocalesPickerDocument }, result: { data: { locales: [] } } },
        {
          request: { query: CreateCmsPageDocument, variables: captureInput(onCreate) },
          result: { data: { createCMSPage: { __typename: 'CMSPage', id: 'p1' } } },
        },
        { request: { query: CmsPagesDocument, variables: () => true }, result: { data: { cmsPages: [] } }, maxUsageCount: 3 },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Create page' }))
    expect(await screen.findByText('Slug is required')).toBeDefined()

    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'about-us' } })
    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'About us' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create page' }))
    expect(await screen.findByText('page editor', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith({ pageTypeId: 'landing', slug: 'about-us', locale: 'en', title: 'About us' })
  })
})
